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

package azure

import (
	"context"
	"fmt"
	"net/url"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/storage/mgmt/storage"
	"github.com/Azure/azure-storage-queue-go/azqueue"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/triggermesh/test-infra/test/e2e/framework"
)

// CreateStorageAccountsClient will create the storage account client
func CreateStorageAccountsClient(subscriptionID string) *storage.AccountsClient {
	storageAccountsClient := storage.NewAccountsClient(subscriptionID)

	auth, err := auth.NewAuthorizerFromEnvironment()
	if err != nil {
		framework.FailfWithOffset(3, "unable to create authorizer: %s", err)
		return nil
	}

	storageAccountsClient.Authorizer = auth

	return &storageAccountsClient
}

// CreateStorageAccount will create the storage account
func CreateStorageAccount(ctx context.Context, cli *storage.AccountsClient, name, rgName, region string) error {
	future, err := cli.Create(ctx, rgName, name, storage.AccountCreateParameters{
		Sku: &storage.Sku{
			Name: storage.SkuNameStandardLRS,
		},
		Kind:                              storage.KindStorage,
		Location:                          to.StringPtr(region),
		AccountPropertiesCreateParameters: &storage.AccountPropertiesCreateParameters{},
	})

	if err != nil {
		framework.FailfWithOffset(3, "unable to create storage account: %s", err)
		return err
	}

	err = future.WaitForCompletionRef(ctx, cli.Client)
	if err != nil {
		framework.FailfWithOffset(3, "unable to complete storage account creation: %s", err)
		return err
	}

	_, err = future.Result(*cli)
	if err != nil {
		framework.FailfWithOffset(3, "storage account creation failed: %s", err)
		return err
	}

	return nil
}

// CreateQueueStorage will create a queue storage message url
func CreateQueueStorage(ctx context.Context, name, accountName string, accountKey string) *azqueue.MessagesURL {
	credential, err := azqueue.NewSharedKeyCredential(accountName, accountKey)
	if err != nil {
		framework.FailfWithOffset(3, "azqueue.NewSharedKeyCredential failed: ", err)
	}

	p := azqueue.NewPipeline(credential, azqueue.PipelineOptions{})

	urlRef, err := url.Parse(fmt.Sprintf("https://%s.queue.core.windows.net", accountName))

	if err != nil {
		framework.FailfWithOffset(3, "url.Parse failed: ", err)
	}

	serviceURL := azqueue.NewServiceURL(*urlRef, p)

	// Create a Queue
	// Create a URL that references a queue in your Azure Storage account.
	// This returns a QueueURL object that wraps the queue's URL and a request pipeline (inherited from serviceURL)
	_, err = serviceURL.NewQueueURL(name).Create(ctx, azqueue.Metadata{}) // Queue names require lowercase
	if err != nil {
		framework.FailfWithOffset(3, "error creating queue: ", err)
	}
	// Create a URL allowing you to manipulate a queue's messages.
	// This returns a MessagesURL object that wraps the queue's messages URL and a request pipeline (inherited from queueURL)

	queueURL := serviceURL.NewQueueURL(name)

	messagesURL := queueURL.NewMessagesURL()

	return &messagesURL

}

// GetStorageAccountKey will return the storage account keys
func GetStorageAccountKey(ctx context.Context, cli *storage.AccountsClient, name, rgName string) (storage.AccountListKeysResult, error) {
	return cli.ListKeys(ctx, rgName, name, storage.ListKeyExpandKerb)
}
