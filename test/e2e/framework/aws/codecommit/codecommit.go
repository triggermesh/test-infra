/*
Copyright (c) 2020 TriggerMesh Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package codecommit

import (
	"errors"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/codecommit"
	"github.com/aws/aws-sdk-go/service/codecommit/codecommitiface"

	"github.com/triggermesh/test-infra/test/e2e/framework"
)

const DefaultBranch = "main"

// CreateRepository creates a CodeCommit repository named after the given
// framework.Framework with an initialized default branch.
func CreateRepository(ccClient codecommitiface.CodeCommitAPI, f *framework.Framework) string /*arn*/ {
	repoName := "e2e-" + f.UniqueName

	repo := &codecommit.CreateRepositoryInput{
		RepositoryName:        aws.String(repoName),
		RepositoryDescription: aws.String("Generated by the TriggerMesh e2e test suite"),
		Tags: aws.StringMap(map[string]string{
			"k8s_namespace": f.UniqueName,
		}),
	}

	repoCreateResp, err := ccClient.CreateRepository(repo)
	if err != nil {
		framework.FailfWithOffset(2, "Failed to create CodeCommit repository: %s", err)
	}

	// initializes the default branch
	CreateCommit(ccClient, repoName)

	return *repoCreateResp.RepositoryMetadata.Arn
}

// DeleteRepository deletes a CodeCommit repository by name.
func DeleteRepository(ccClient codecommitiface.CodeCommitAPI, name string) {
	repo := &codecommit.DeleteRepositoryInput{
		RepositoryName: aws.String(name),
	}

	if _, err := ccClient.DeleteRepository(repo); err != nil {
		framework.FailfWithOffset(2, "Failed to delete CodeCommit repository: %s", err)
	}
}

// CreateCommit creates a Git commit on the default branch in the repository
// with the given name.
func CreateCommit(ccClient codecommitiface.CodeCommitAPI, repoName string) {
	commit := &codecommit.CreateCommitInput{
		RepositoryName: &repoName,
		BranchName:     aws.String(DefaultBranch),
		AuthorName:     aws.String("TriggerMesh e2e"),
		Email:          aws.String("dev@triggermesh.com"),

		PutFiles: []*codecommit.PutFileEntry{{
			FilePath:    aws.String("README.md"),
			FileContent: []byte("File updated at " + time.Now().Format(time.RFC3339Nano)),
		}},
	}

	// verify that the branch already exists, in which case the
	// ParentCommitId field must be set
	br, err := ccClient.GetBranch(&codecommit.GetBranchInput{
		RepositoryName: &repoName,
		BranchName:     commit.BranchName,
	})
	switch {
	case isBranchNotFound(err):
		// no parent commit
	case err != nil:
		framework.FailfWithOffset(2, "Failed to retrieve branch information: %s", err)
	default:
		commit.ParentCommitId = br.Branch.CommitId
	}

	if _, err := ccClient.CreateCommit(commit); err != nil {
		framework.FailfWithOffset(2, "Failed to create Git commit: %s", err)
	}
}

// isBranchNotFound returns whether the given error indicates that a Git branch
// was not found.
func isBranchNotFound(err error) bool {
	if awsErr := awserr.Error(nil); errors.As(err, &awsErr) {
		return awsErr.Code() == codecommit.ErrCodeBranchDoesNotExistException
	}
	return false
}
