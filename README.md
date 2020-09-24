# End-to-end test suite

This is the entry point of the TriggerMesh E2E test suite.

## Contents

  - [Overview](#overview)
  - [Running tests](#running-tests)
    - [Execution](#execution)
    - [Inputs](#inputs)
  - [Package organization](#package-organization)
  - [Writing tests](#writing-tests)
    - [Structure](#structure)
    - [Best practices](#best-practices)
      - [Scoped variables](#scoped-variables)
      - [Scoped setup with `BeforeEach`](#scoped-setup-with-beforeeach)
      - [Fail within helpers](#fail-within-helpers)

## Overview

This package contains a single Go test called `TestE2e` which runs all the test specs contained in its sub-packages.

Each test is written using the [Ginkgo][ginkgo-docs] testing framework using Behavior-Driven Development style ("BDD").

The [`framework.Famework`](./framework/) abstraction, heavily inspired by [Kubernetes' E2E tests][k8s-e2e], offers a
convenient way for each test spec to run against its own short-lived Kubernetes namespace, with common setup and cleanup
tasks executed automatically before and after each of these specs.

## Running tests

### Execution

While it is possible to run tests using `go test` like standard unit tests, it is recommended to use the `ginkgo` CLI
tool for running Ginkgo test suites, which offers better control over Gingko-specific parameters, such as the
parallelism of test specs, the format and verbosity of the reporter's output, etc.

```sh
# Using the Ginkgo version pinned inside `go.mod`
go run github.com/onsi/ginkgo/ginkgo

# Or using an executable installed via `go get`
ginkgo
```

The `e2e` Make goal, available at the root of this repository, wraps the above command together with sane default settings. If the location of the `kubeconfig` file lies in a directory outside of the default,`/$USER/.kube/config`, the Makefile will need to be updated with it's location.

### Inputs

All tests require a `kubeconfig` file containing credentials of a user or service account with elevated permissions to
interact with a Kubernetes cluster, unless tests run in a Kubernetes pod, in which case the pod's service account is
used as a fallback. The path to this file can be set using the `-e2e.kubeconfig` flag, which defaults to the value of
the `KUBECONFIG` environment variable.

Some tests require more specific input, such as access tokens to interact with third-party APIs (AWS, GitHub, etc.). The
input method for those tests varies depending on the client that is used. For example, the Go client for AWS reads
security credentials from the environment and falls back to the standard location of a local file containing shared
credentials ([ref.][aws-go-session]), while the GitHub client expects an OAuth2 access token to be exported in the
environment ([ref.][gh-client]). Please refer to documentation of each test for a description of the required inputs.
Below is an example of such documentation:

```go
/* This test suite requires:

   - AWS credentials in whichever form (https://docs.aws.amazon.com/sdk-for-go/api/aws/session/#hdr-Sessions_options_from_Shared_Config)
   - The name of an AWS region exported in the environment as AWS_REGION
*/
```

The `Makefile` also outlines the required environment variables.

## Package organization

Each subdirectory of the `test/e2e` package contains a series of tests organized by category (e.g. _Sources_, _Targets_,
_Bridges_, ...), with the exception of the `framework` package which contains the test framework itself, along with all
the test helpers imported by actual tests.

```
test/e2e
├── bridges
│   ├── somebridge
│   ├── otherbridge
│   └── ...
├── sources
│   ├── somesource
│   ├── othersource
│   └── ...
├── targets
│   ├── sometarget
│   ├── othertarget
│   └── ...
└── framework
    ├── aws
    ├── bridges
    ├── ducktypes
    ├── github
    └── ...
```

We favour creating sub-packages within those top-level category packages whenever a given test may have helper functions
which natural name could conflict with other tests. For instance, tests for an event source _"foo"_ may declare a
`createSource` helper which name potentially conflicts with a similar helper declared in the tests for an event source
_"bar"_. Requiring the developer to avoid that situation by creating each helper function with a differentiator in the
name, such as `createFooSource` or `createBarSource`, would be verbose and counter-productive.

Whenever multiple tests could benefit from shared helpers and those can be written generically, it is recommended to
move those helpers to a suitable sub-package of `framework`.

## Writing tests

### Structure

The Ginkgo documentation contains good examples for [Structuring Your Specs][ginkgo-struct] in an expressive manner.

Ginkgo spec blocks can be nested in many different ways and developers are free to organize them however they please.
One golden rule is that **the hierarchy of spec blocks should read as naturally as possible**.

Here are a few tips that can help achieving the above goal:

* The name of `Describe` blocks should be brief and describe _what_ is being tested.

    ```go
    Describe("Amazon S3 event target", func() {
        Describe("status conditions", func() {
        })
    })
    ```

* `Context` and `When` blocks describe under what circumstances the _what_ is being tested, and are generally nested
  under `Describe` blocks.

    ```go
    Describe("Integration bridge", func() {
        When("all components becomes ready", func() {
        })

        Context("an event is sent to the broker", func() {
        })
    })
    ```

    Those blocks _should not_ contain any code besides variable declarations, if relevant to the context.

* `It` blocks contain the actual assertions.

    ```go
    Describe("Slack source", func() {
        When("someone writes in the #music channel", func() {
            It("should play a loud sound", func() {
                Expect(volume).To(BeLoud())
            })

            It("should notify Pablo", func() {
                msg := slackBotMessagesToUser("pablo")
                Expect(msg).To(Contain("some cool jam for you"))
            })

            It("should report a copyright infringement", func() {
            })
        })
    })
    ```

    Those blocks can be located within any of the blocks described above, including directly under a `Describe` when the
    description of the `It` block contains enough context on its own.

    ```go
    Describe("GitHub source webhook", func() {
        It(`should set "main" as the default branch`, func() {
        })
    })
    ```

* `By` blocks are purely cosmetic but help splitting some complex logic into multiple, easy to identify steps. This is
  particularly useful in `BeforeEach` blocks.

    ```go
    BeforeEach(func() {
        By("creating a client", func() {
            client = NewClient()
        })

        By("initializing something", func() {
            client.DoSomething()
        })
    })
    ```

* While it is possible to create multiple levels of nesting of `Describe`, `Context` and `When` blocks with no technical
  restriction, the aim should be to keep all assertions within at most **3** levels of nesting for optimal readability.

### Best practices

The best practices outlined in the [Writing good e2e tests for Kubernetes][sigtesting-howto] document apply to our own
tests.

A few more recommendations and pitfalls that can be easily avoided are described below.

#### Scoped variables

One of the most important concepts of Ginkgo tests is the scoping of variables.

All variables declared within a `Describe`, `Context` or `When` block are cloned during the execution of an `It` block,
meaning that the value of a closure variable set in a given `It` will not affect the state of another `It`, regardless
of the order in which those blocks execute, or whether they execute in parallel.

```go
Describe("Variable scope", func() {
    var text string

    When(`two "It" modify the "text" closure variable`, func() {
        It("writes some value", func() {
            text = "foo"
        })

        It("writes another value", func() {
            text = "bar"
        })

        It("reads the value", func() {
            framework.Logf(text) // always prints ""
        })
    })
})
```

This is particularly useful to share state between `BeforeEach` and `It` blocks. Typically, `BeforeEach` blocks
initialize the value of closure variables, while `It` blocks are responsible for performing assertions on/using those
initialized variables.

```go
Describe("Variable scope", func() {
    var jsonInput string

    When("input is invalid", func() {
        BeforeEach(func() {
            jsonInput = "oops this is invalid"
        })

        It("fails to parse", func() {
            err := json.Parse(jsonInput)
            Expect(err).To(HaveOccured())
        })
    })

    When("input is valid", func() {
        BeforeEach(func() {
            jsonInput = `{"valid": true}`
        })

        It("parses successfully", func() {
            err := json.Parse(jsonInput)
            Expect(err).ToNot(HaveOccured())
        })
    })
})
```

Closure variables are typically defined at the top of the most relevant `Describe`, `Context` or `When` block.

#### Scoped setup with `BeforeEach`

The logic contained in a `BeforeEach` block is executed **once per `It` block** contained within the same `Describe`,
`Context` or `When` block, including at lower levels of `Describe`, `Context` and `When`.

This can influence the way tests are structured. For instance, if tests within a given `Context` block require the
initialization of a dependency while other tests within a different `Context` do not require that dependency, it is wise
to avoid placing initialization steps (and variables) specific to the former directly under the global `Describe`.

```go
Describe("Greeting", func() {
    // available to the current "Describe" and *all* its sub-blocks
    var name string

    // executed in "It"s of the current "Describe" and *all** its sub-blocks
    BeforeEach(func() {
        name = os.User()
    })

    When("no weather forecast is available", func() {
        It("greets without the weather", func() {
            Expect(Greet(nil)).To(Equal("Hello " + name))
        })
    })

    When("a weather forecast is available", func() {
        // available only to the current "When" and its sub-blocks
        var forecast *weather.Forecast

        // executed only in "It"s of the current "When" and its sub-blocks
        BeforeEach(func() {
            forecast = weather.GetWeather
        })

        It("greets with the weather", func() {
            Expect(Greet(forecast)).To(Equal(
                fmt.Sprintf("Hello %s, the weather will be %s today", name, forecast)
            ))
        })
    })
})
```

Similarly to closure variables, `BeforeEach` blocks are typically defined at the top of the most relevant `Describe`,
`Context` or `When` block.

**:information_source: In the context of our framework, a new instance of `framework.Framework`, and therefore a new
Kubernetes namespace, is created for each `It` block.** Consider this when writing tests that create API objects which
take a long time to set up. Here is an [example test][optimized-test] optimized to run each of its specs in the same
namespace.

#### Fail within helpers

Keeping the number of lines of code to a minimum within the main test body helps focusing on the actual logic. Resorting
to helper functions is a great way to achieve this, and handling errors directly within those helpers is equally
important.

Generally speaking, if a function returns an error along with other values and we don't expect an error to occur within
tests, that function can be wrapped inside a helper which only returns the values that are meant to be used in tests.
Instead of leaving the error handling to `It` or `BeforeEach` blocks, the helper can call `framework.Failf` whenever an
error occurs and immediately fail the current test.

```go
// okay-ish

var _ = Describe("Error handling", func() {
    It("does something and checks the output", func() {
        something, err := mypkg.GetSomething()
        Expect(err).ToNot(HaveOccured())

        output, err := mypkg.HandleSomething(something)
        Expect(err).ToNot(HaveOccured())
        Expect(output).ToNot(BeNil())
    })
})
```

```go
// better

var _ = Describe("Error handling", func() {
    It("does something and checks the output", func() {
        Expect(doSomething()).ToNot(BeNil())
    })
})

func doSomething() *mypkg.Output {
    something, err := mypkg.GetSomething()
    if err != nil {
        framework.Failf("Failed to get something: %s", err)
    }

    output, err := mypkg.HandleSomething(something)
    if err != nil {
        framework.Failf("Failed to handle something: %s", err)
    }

    return output
}
```

The benefits of this approach become more obvious when the complexity of test scenarios grows beyond the example above.


[ginkgo-docs]: https://onsi.github.io/ginkgo/
[ginkgo-struct]: https://onsi.github.io/ginkgo/#structuring-your-specs
[optimized-test]: https://github.com/triggermesh/test-infra/blob/956c8ce257/test/e2e/sources/awscodecommit/main.go#L172-L188

[k8s-e2e]: https://godoc.org/k8s.io/kubernetes/test/e2e
[sigtesting-howto]: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-testing/writing-good-e2e-tests.md

[aws-go-session]: https://docs.aws.amazon.com/sdk-for-go/api/aws/session/
[gh-client]: https://github.com/triggermesh/test-infra/blob/9f19ed28a9/test/e2e/framework/github/github.go#L39-L53
