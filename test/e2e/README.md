# README

Create a AWS user for the tests with the permissions listed in `aws_iam_policy.jsonc`

Export the following environment variables and set their values accordingly:

|   Environment Variable  |    Description    |
|-------------------------|-------------------|
| `AWS_ACCESS_KEY_ID`     | Access key ID     |
| `AWS_SECRET_ACCESS_KEY` | Secret access key |
| `AWS_REGION`            | AWS Region        |

Ensure the `kubectl` user running the tests has access to the cluster on which the tests need to be executed and has the aws event sources deployed.

To run the tests, execute the following command:

```bash
$ go run github.com/onsi/ginkgo/ginkgo -nodes=4 -slowSpecThreshold=60 -randomizeAllSpecs ./ -- -e2e.kubeconfig=${HOME}/.kube/config
```
