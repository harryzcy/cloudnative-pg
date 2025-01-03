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

package e2e

import (
	"encoding/json"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/reconciler/persistentvolumeclaim"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"
	"github.com/cloudnative-pg/cloudnative-pg/tests"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/clusterutils"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/objects"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/postgres"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/run"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/storage"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/timeouts"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/yaml"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Cluster Hibernation with plugin", Label(tests.LabelPlugin), func() {
	type mode string
	type hibernateSatusMessage string
	type expectedKeysInStatus string
	const (
		sampleFileClusterWithPGWalVolume    = fixturesDir + "/base/cluster-storage-class.yaml.template"
		sampleFileClusterWithOutPGWalVolume = fixturesDir + "/hibernate/" +
			"cluster-storage-class-without-wal.yaml.template"
		level                                         = tests.Medium
		HibernateOn             mode                  = "on"
		HibernateOff            mode                  = "off"
		HibernateStatus         mode                  = "status"
		clusterOffStatusMessage hibernateSatusMessage = "No Hibernation. Cluster Deployed."
		clusterOnStatusMessage  hibernateSatusMessage = "Cluster Hibernated"
		summaryInStatus         expectedKeysInStatus  = "summary"
		tableName                                     = "test"
	)
	BeforeEach(func() {
		if testLevelEnv.Depth < int(level) {
			Skip("Test depth is lower than the amount requested for this test")
		}
	})

	Context("hibernate", func() {
		var namespace string
		var err error
		getPrimaryAndClusterManifest := func(namespace, clusterName string) ([]byte, string) {
			var beforeHibernationClusterInfo *apiv1.Cluster
			var clusterManifest []byte
			var beforeHibernationCurrentPrimary string
			By("collecting current primary details", func() {
				beforeHibernationClusterInfo, err = clusterutils.Get(env.Ctx, env.Client, namespace, clusterName)
				Expect(err).ToNot(HaveOccurred())
				beforeHibernationCurrentPrimary = beforeHibernationClusterInfo.Status.CurrentPrimary
				// collect expected cluster manifesto info
				clusterManifest, err = json.Marshal(&beforeHibernationClusterInfo)
				Expect(err).ToNot(HaveOccurred())
			})
			return clusterManifest, beforeHibernationCurrentPrimary
		}
		getPvc := func(role persistentvolumeclaim.Meta, instanceName string) corev1.PersistentVolumeClaim {
			pvcName := role.GetName(instanceName)
			pvcInfo := corev1.PersistentVolumeClaim{}
			err = objects.Get(env.Ctx, env.Client, ctrlclient.ObjectKey{Namespace: namespace, Name: pvcName}, &pvcInfo)
			Expect(err).ToNot(HaveOccurred())
			return pvcInfo
		}
		performHibernation := func(mode mode, namespace, clusterName string) {
			By(fmt.Sprintf("performing hibernation %v", mode), func() {
				_, _, err := run.Run(fmt.Sprintf("kubectl cnpg hibernate %v %v -n %v",
					mode, clusterName, namespace))
				Expect(err).ToNot(HaveOccurred())
			})
			By(fmt.Sprintf("verifying cluster %v pods are removed", clusterName), func() {
				Eventually(func(g Gomega) {
					podList, _ := clusterutils.ListPods(env.Ctx, env.Client, namespace, clusterName)
					g.Expect(len(podList.Items)).Should(BeEquivalentTo(0))
				}, 300).Should(Succeed())
			})
		}

		getHibernationStatusInJSON := func(namespace, clusterName string) map[string]interface{} {
			var data map[string]interface{}
			By("getting hibernation status", func() {
				stdOut, _, err := run.Run(fmt.Sprintf("kubectl cnpg hibernate %v %v -n %v -ojson",
					HibernateStatus, clusterName, namespace))
				Expect(err).ToNot(HaveOccurred(), stdOut)
				err = json.Unmarshal([]byte(stdOut), &data)
				Expect(err).ToNot(HaveOccurred())
			})
			return data
		}

		verifySummaryInHibernationStatus := func(clusterName string, message hibernateSatusMessage) {
			statusOut := getHibernationStatusInJSON(namespace, clusterName)
			actualStatus := statusOut[string(summaryInStatus)].(map[string]interface{})["status"].(string)
			Expect(strings.Contains(string(message), actualStatus)).Should(BeEquivalentTo(true),
				actualStatus+"\\not-contained-in\\"+string(message))
		}
		verifyClusterResources := func(
			namespace, clusterName string, objs []persistentvolumeclaim.ExpectedObjectCalculator,
		) {
			By(fmt.Sprintf("verifying cluster resources are removed "+
				"post hibernation where roles %v", objs), func() {
				timeout := 120

				By(fmt.Sprintf("verifying cluster %v is removed", clusterName), func() {
					Eventually(func() (bool, apiv1.Cluster) {
						cluster, err := clusterutils.Get(env.Ctx, env.Client, namespace, clusterName)
						if err != nil {
							return true, apiv1.Cluster{}
						}
						return false, *cluster
					}, timeout).Should(BeTrue())
				})

				By(fmt.Sprintf("verifying cluster %v PVCs are removed", clusterName), func() {
					Eventually(func() (int, error) {
						pvcList, err := storage.GetPVCList(env.Ctx, env.Client, namespace)
						if err != nil {
							return -1, err
						}
						return len(pvcList.Items), nil
					}, timeout).Should(BeEquivalentTo(len(objs)))
				})

				By(fmt.Sprintf("verifying cluster %v configMap is removed", clusterName), func() {
					Eventually(func() (bool, corev1.ConfigMap) {
						configMap := corev1.ConfigMap{}
						err = env.Client.Get(env.Ctx,
							ctrlclient.ObjectKey{Namespace: namespace, Name: apiv1.DefaultMonitoringConfigMapName},
							&configMap)
						if err != nil {
							return true, corev1.ConfigMap{}
						}
						return false, configMap
					}, timeout).Should(BeTrue())
				})

				By(fmt.Sprintf("verifying cluster %v secrets are removed", clusterName), func() {
					Eventually(func() (bool, corev1.SecretList, error) {
						secretList := corev1.SecretList{}
						err = env.Client.List(env.Ctx, &secretList, ctrlclient.InNamespace(namespace))
						if err != nil {
							return false, corev1.SecretList{}, err
						}
						var getClusterSecrets []string
						for _, secret := range secretList.Items {
							if strings.HasPrefix(secret.GetName(), clusterName) {
								getClusterSecrets = append(getClusterSecrets, secret.GetName())
							}
						}
						if len(getClusterSecrets) == 0 {
							return true, corev1.SecretList{}, nil
						}
						return false, secretList, nil
					}, timeout).Should(BeTrue())
				})

				By(fmt.Sprintf("verifying cluster %v role is removed", clusterName), func() {
					Eventually(func() (bool, v1.Role) {
						role := v1.Role{}
						err = env.Client.Get(env.Ctx,
							ctrlclient.ObjectKey{Namespace: namespace, Name: clusterName},
							&role)
						if err != nil {
							return true, v1.Role{}
						}
						return false, role
					}, timeout).Should(BeTrue())
				})

				By(fmt.Sprintf("verifying cluster %v rolebinding is removed", clusterName), func() {
					Eventually(func() (bool, v1.RoleBinding) {
						roleBinding := v1.RoleBinding{}
						err = env.Client.Get(env.Ctx,
							ctrlclient.ObjectKey{Namespace: namespace, Name: clusterName},
							&roleBinding)
						if err != nil {
							return true, v1.RoleBinding{}
						}
						return false, roleBinding
					}, timeout).Should(BeTrue())
				})
			})
		}
		verifyPvc := func(
			expectedObject persistentvolumeclaim.ExpectedObjectCalculator, pvcUid types.UID,
			clusterManifest []byte, instanceName string,
		) {
			pvcInfo := getPvc(expectedObject, instanceName)
			Expect(pvcUid).Should(BeEquivalentTo(pvcInfo.GetUID()))
			// pvc should be attached annotation with pgControlData and Cluster manifesto
			expectedAnnotationKeyPresent := []string{
				utils.HibernatePgControlDataAnnotationName,
				utils.HibernateClusterManifestAnnotationName,
				utils.PgControldataAnnotationName,
				utils.ClusterManifestAnnotationName,
			}
			storage.ObjectHasAnnotations(&pvcInfo, expectedAnnotationKeyPresent)
			expectedAnnotation := map[string]string{
				utils.HibernateClusterManifestAnnotationName: string(clusterManifest),
				utils.ClusterManifestAnnotationName:          string(clusterManifest),
			}
			storage.ObjectMatchesAnnotations(&pvcInfo, expectedAnnotation)
		}

		assertHibernation := func(namespace, clusterName, tableName string) {
			var beforeHibernationPgWalPvcUID types.UID
			var beforeHibernationPgDataPvcUID types.UID

			// Write a table and some data on the "app" database
			tableLocator := TableLocator{
				Namespace:    namespace,
				ClusterName:  clusterName,
				DatabaseName: postgres.AppDBName,
				TableName:    tableName,
			}
			AssertCreateTestData(env, tableLocator)
			clusterManifest, currentPrimary := getPrimaryAndClusterManifest(namespace, clusterName)

			By("collecting pgWal pvc details of current primary", func() {
				pvcInfo := getPvc(persistentvolumeclaim.NewPgWalCalculator(), currentPrimary)
				beforeHibernationPgWalPvcUID = pvcInfo.GetUID()
			})

			By("collecting pgData pvc details of current primary", func() {
				pvcInfo := getPvc(persistentvolumeclaim.NewPgDataCalculator(), currentPrimary)
				beforeHibernationPgDataPvcUID = pvcInfo.GetUID()
			})

			By(fmt.Sprintf("verifying hibernation status"+
				" before hibernate on cluster %v", clusterName), func() {
				verifySummaryInHibernationStatus(clusterName, clusterOffStatusMessage)
			})

			performHibernation(HibernateOn, namespace, clusterName)

			By(fmt.Sprintf("verifying hibernation status"+
				" after hibernate on cluster %v", clusterName), func() {
				verifySummaryInHibernationStatus(clusterName, clusterOnStatusMessage)
			})

			// After hibernation, it will destroy all the resources generated by the cluster,
			// except the PVCs that belong to the PostgreSQL primary instance.
			verifyClusterResources(
				namespace,
				clusterName,
				[]persistentvolumeclaim.ExpectedObjectCalculator{
					persistentvolumeclaim.NewPgWalCalculator(),
					persistentvolumeclaim.NewPgDataCalculator(),
				},
			)

			By("verifying primary pgWal pvc info", func() {
				verifyPvc(
					persistentvolumeclaim.NewPgWalCalculator(),
					beforeHibernationPgWalPvcUID,
					clusterManifest,
					currentPrimary,
				)
			})

			By("verifying primary pgData pvc info", func() {
				verifyPvc(
					persistentvolumeclaim.NewPgDataCalculator(),
					beforeHibernationPgDataPvcUID,
					clusterManifest,
					currentPrimary,
				)
			})

			// verifying hibernation off
			performHibernation(HibernateOff, namespace, clusterName)

			By(fmt.Sprintf("verifying hibernation status after "+
				"perform hibernation off on cluster %v", clusterName), func() {
				verifySummaryInHibernationStatus(clusterName, clusterOffStatusMessage)
			})

			AssertClusterIsReady(namespace, clusterName, testTimeouts[timeouts.ClusterIsReady], env)
			// Test data should be present after hibernation off
			AssertDataExpectedCount(env, tableLocator, 2)
		}

		When("cluster setup with PG-WAL volume", func() {
			It("hibernation process should work", func() {
				const namespacePrefix = "hibernation-on-with-pg-wal"
				clusterName, err := yaml.GetResourceNameFromYAML(env.Scheme, sampleFileClusterWithPGWalVolume)
				Expect(err).ToNot(HaveOccurred())
				// Create a cluster in a namespace we'll delete after the test
				namespace, err = env.CreateUniqueTestNamespace(env.Ctx, env.Client, namespacePrefix)
				Expect(err).ToNot(HaveOccurred())
				AssertCreateCluster(namespace, clusterName, sampleFileClusterWithPGWalVolume, env)
				assertHibernation(namespace, clusterName, tableName)
			})
		})
		When("cluster setup without PG-WAL volume", func() {
			It("hibernation process should work", func() {
				var beforeHibernationPgDataPvcUID types.UID

				const namespacePrefix = "hibernation-without-pg-wal"
				clusterName, err := yaml.GetResourceNameFromYAML(env.Scheme, sampleFileClusterWithOutPGWalVolume)
				Expect(err).ToNot(HaveOccurred())
				// Create a cluster in a namespace we'll delete after the test
				namespace, err = env.CreateUniqueTestNamespace(env.Ctx, env.Client, namespacePrefix)
				Expect(err).ToNot(HaveOccurred())
				AssertCreateCluster(namespace, clusterName, sampleFileClusterWithOutPGWalVolume, env)
				// Write a table and some data on the "app" database
				tableLocator := TableLocator{
					Namespace:    namespace,
					ClusterName:  clusterName,
					DatabaseName: postgres.AppDBName,
					TableName:    tableName,
				}
				AssertCreateTestData(env, tableLocator)
				clusterManifest, currentPrimary := getPrimaryAndClusterManifest(namespace,
					clusterName)

				By("collecting pgData pvc details of current primary", func() {
					pvcInfo := getPvc(persistentvolumeclaim.NewPgDataCalculator(), currentPrimary)
					beforeHibernationPgDataPvcUID = pvcInfo.GetUID()
				})

				By(fmt.Sprintf("verifying hibernation status"+
					" before hibernate on cluster %v", clusterName), func() {
					verifySummaryInHibernationStatus(clusterName, clusterOffStatusMessage)
				})

				performHibernation(HibernateOn, namespace, clusterName)

				By(fmt.Sprintf("verifying hibernation status"+
					" after hibernate on cluster %v", clusterName), func() {
					verifySummaryInHibernationStatus(clusterName, clusterOnStatusMessage)
				})

				// After hibernation, it will destroy all the resources generated by the cluster,
				// except the PVCs that belong to the PostgreSQL primary instance.
				verifyClusterResources(
					namespace,
					clusterName,
					[]persistentvolumeclaim.ExpectedObjectCalculator{persistentvolumeclaim.NewPgDataCalculator()},
				)

				By("verifying primary pgData pvc info", func() {
					verifyPvc(
						persistentvolumeclaim.NewPgDataCalculator(),
						beforeHibernationPgDataPvcUID,
						clusterManifest,
						currentPrimary,
					)
				})

				// verifying hibernation off
				performHibernation(HibernateOff, namespace, clusterName)
				By(fmt.Sprintf("verifying hibernation status"+
					" before hibernate on cluster %v", clusterName), func() {
					verifySummaryInHibernationStatus(clusterName, clusterOffStatusMessage)
				})

				AssertClusterIsReady(namespace, clusterName, testTimeouts[timeouts.ClusterIsReady], env)
				// Test data should be present after hibernation off
				AssertDataExpectedCount(env, tableLocator, 2)
			})
		})
		When("cluster hibernation after switchover", func() {
			It("hibernation process should work", func() {
				const namespacePrefix = "hibernation-with-switchover"
				clusterName, err := yaml.GetResourceNameFromYAML(env.Scheme, sampleFileClusterWithPGWalVolume)
				Expect(err).ToNot(HaveOccurred())
				// Create a cluster in a namespace we'll delete after the test
				namespace, err = env.CreateUniqueTestNamespace(env.Ctx, env.Client, namespacePrefix)
				Expect(err).ToNot(HaveOccurred())
				AssertCreateCluster(namespace, clusterName, sampleFileClusterWithPGWalVolume, env)
				AssertSwitchover(namespace, clusterName, env)
				assertHibernation(namespace, clusterName, tableName)
			})
		})
	})
})
