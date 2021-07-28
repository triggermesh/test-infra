/*
Copyright (c) 2021 TriggerMesh Inc.

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

package gitlab

import (
	"os"
	"time"

	"github.com/triggermesh/test-infra/test/e2e/framework"
	gogitlab "github.com/xanzy/go-gitlab"
)

const (
	DefaultBranch = "main"
	DefaultFile   = "README.md"
	DefaultAuthor = "TriggerMesh e2e user"
	DefaultAuthorEmail = "dev@triggermesh.com"
	DefaultBaseURL = "https://gitlab.com/triggermeshDev/"
	DefaultSecretToken = "test12345" // something unique for this interaction
)

type GitlabHandle struct  {
	client *gogitlab.Client
}

// NewClient returns a GitLab client
func NewClient(token string) (*GitlabHandle, error) {
	client, err := gogitlab.NewClient(token)

	return &GitlabHandle{
		client: client,
	}, err
}

func (h *GitlabHandle) CreateProject(uniqueName string) (*gogitlab.Project, error) {
	name := "e2e-" + uniqueName
	description := "Generated by the TriggerMesh e2e test suite"

	projOpt := &gogitlab.CreateProjectOptions{
		Name:                             &name,
		Description: &description,
	}

	proj, _, err := h.client.Projects.CreateProject(projOpt)

	return proj, err
}

func (h *GitlabHandle) DeleteProject(project *gogitlab.Project) error {
	_, err := h.client.Projects.DeleteProject(project.ID)

	return err
}

func (h *GitlabHandle) CreateCommit(project *gogitlab.Project) *gogitlab.File {
	author := DefaultAuthor
	authorEmail := DefaultAuthorEmail
	content := "At the sound of the tone, the current time will be " + time.Now().Format(time.RFC1123)
	branch := DefaultBranch
	commitMsg := "Initial commit test"

	commitOpts := gogitlab.CreateFileOptions{
		Branch:        &branch,
		AuthorEmail:   &authorEmail,
		AuthorName:    &author,
		Content:       &content,
		CommitMessage: &commitMsg,
	}

	_, _, err := h.client.RepositoryFiles.CreateFile(project.ID, DefaultFile, &commitOpts)
	if err != nil {
		framework.FailfWithOffset(2, "Failed to create file: %s", err)
	}

	ref := "refs/heads/" + DefaultBranch

	file, _, err := h.client.RepositoryFiles.GetFileMetaData(project.ID, DefaultFile, &gogitlab.GetFileMetaDataOptions{
		Ref: &ref,
	})

	if err != nil {
		framework.FailfWithOffset(2, "Failed to find file: %s", err)
	}

	return file
}

func GetToken() string {
	return os.Getenv("GITLAB_API_TOKEN")
}