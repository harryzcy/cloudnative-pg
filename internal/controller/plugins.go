/*
Copyright The CloudNativePG Contributors

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

package controller

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	cnpgiClient "github.com/cloudnative-pg/cloudnative-pg/internal/cnpi/plugin/client"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"
)

// preReconcilePluginHooks ensures we call the pre-reconcile plugin hooks
func preReconcilePluginHooks(
	ctx context.Context,
	cluster *apiv1.Cluster,
	object client.Object,
) cnpgiClient.ReconcilerHookResult {
	pluginClient := getPluginClientFromContext(ctx)
	return pluginClient.PreReconcile(ctx, cluster, object)
}

// postReconcilePluginHooks ensures we call the post-reconcile plugin hooks
func postReconcilePluginHooks(
	ctx context.Context,
	cluster *apiv1.Cluster,
	object client.Object,
) cnpgiClient.ReconcilerHookResult {
	pluginClient := getPluginClientFromContext(ctx)
	return pluginClient.PostReconcile(ctx, cluster, object)
}

// setPluginClientInContext records the plugin client in the given context
func setPluginClientInContext(ctx context.Context, client cnpgiClient.Client) context.Context {
	return context.WithValue(ctx, utils.PluginClientKey, client)
}

// getPluginClientFromContext gets the current plugin client from the context
func getPluginClientFromContext(ctx context.Context) cnpgiClient.Client {
	return ctx.Value(utils.PluginClientKey).(cnpgiClient.Client)
}
