package services

import (
	"fmt"
	"time"

	"github.com/cloudfoundry-incubator/cf-test-helpers/cf"
	"github.com/cloudfoundry-incubator/cf-test-helpers/generator"
	"github.com/cloudfoundry/cf-acceptance-tests/helpers/assets"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
)

type LastOperation struct {
	State string `json:"state"`
}

type Service struct {
	Name          string        `json:"name"`
	LastOperation LastOperation `json:"last_operation"`
}

type Resource struct {
	Entity Service `json:"entity"`
}

type Response struct {
	Resources []Resource `json:"resources"`
}

var _ = Describe("Service Instance Lifecycle", func() {
	var broker ServiceBroker

	waitForAsyncDeletionToComplete := func(broker ServiceBroker, instanceName string) {
		Eventually(func() string {
			serviceDetails := cf.Cf("service", instanceName).Wait(DEFAULT_TIMEOUT)
			return string(serviceDetails.Out.Contents())
		}, 5*time.Minute, time.Second).Should(ContainSubstring("not found"), "broker deletion did not complete")
	}

	waitForAsyncOperationToComplete := func(broker ServiceBroker, instanceName string) {
		Eventually(func() string {
			serviceDetails := cf.Cf("service", instanceName)
			Eventually(serviceDetails, DEFAULT_TIMEOUT).Should(Exit(0), "failed getting service instance details")
			return string(serviceDetails.Out.Contents())
		}, 5*time.Minute, time.Second).Should(ContainSubstring("succeeded"), "service check did not succeed")
	}

	Context("Sync broker", func() {
		BeforeEach(func() {
			broker = NewServiceBroker(generator.RandomName(), assets.NewAssets().ServiceBroker, context)
			broker.Push()
			broker.Configure()
			broker.Create()
			broker.PublicizePlans()
		})

		AfterEach(func() {
			broker.Destroy()
		})

		Context("just service instances", func() {
			It("can create a service instance", func() {
				instanceName := generator.RandomName()
				createService := cf.Cf("create-service", broker.Service.Name, broker.SyncPlans[0].Name, instanceName)
				Eventually(createService, DEFAULT_TIMEOUT).Should(Exit(0), "failed to create a service instance")

				serviceInfo := cf.Cf("service", instanceName)
				Eventually(serviceInfo.Out.Contents(), DEFAULT_TIMEOUT).Should(ContainSubstring(fmt.Sprintf("Plan: %s", broker.SyncPlans[0].Name)),
					"service instance did not contain plan")
			})

			Context("service instance exists", func() {
				var instanceName string

				BeforeEach(func() {
					instanceName = generator.RandomName()
					createService := cf.Cf("create-service", broker.Service.Name, broker.SyncPlans[0].Name, instanceName)
					Eventually(createService, DEFAULT_TIMEOUT).Should(Exit(0), "failed to create service instance")
				})

				FIt("can update a service instance", func() {
					updateService := cf.Cf("update-service", instanceName, "-p", broker.SyncPlans[1].Name)
					Eventually(updateService, DEFAULT_TIMEOUT).Should(Exit(0), "failed to update the service instance")

					Eventually(func() string {
						serviceInfo := cf.Cf("service", instanceName)
						contents := string(serviceInfo.Out.Contents())
						fmt.Println(contents)
						return contents
					}, DEFAULT_TIMEOUT).Should(ContainSubstring(fmt.Sprintf("Plan: %s", broker.SyncPlans[1].Name)),
						"service instance did not contain plan")
				})

				It("can delete a service instance", func() {
					deleteService := cf.Cf("delete-service", instanceName, "-f")
					Eventually(deleteService, DEFAULT_TIMEOUT).Should(Exit(0), "failed to delete service instance")

					serviceInfo := cf.Cf("service", instanceName)
					Eventually(serviceInfo.Out.Contents(), DEFAULT_TIMEOUT).Should(ContainSubstring("not found"),
						"delete failed")
				})
			})
		})

		Context("service instances with an app", func() {
			var appName string
			var instanceName string

			BeforeEach(func() {
				appName = generator.RandomName()
				createApp := cf.Cf("push", appName, "-p", assets.NewAssets().Dora)
				Eventually(createApp, CF_PUSH_TIMEOUT).Should(Exit(0), "failed creating app")

				checkForEvents(appName, []string{"audit.app.create"})

				instanceName = generator.RandomName()
				createService := cf.Cf("create-service", broker.Service.Name, broker.SyncPlans[0].Name, instanceName)
				Eventually(createService, DEFAULT_TIMEOUT).Should(Exit(0), "failed creating service")
			})
			AfterEach(func() {
				deleteService := cf.Cf("delete-service", instanceName, "-f")
				Eventually(deleteService, DEFAULT_TIMEOUT).Should(Exit(0), "deletion failed")
			})

			It("can bind service to an app", func() {
				bindService := cf.Cf("bind-service", appName, instanceName)
				Eventually(bindService, DEFAULT_TIMEOUT).Should(Exit(0), "failed binding app to service")

				checkForEvents(appName, []string{"audit.app.update"})

				restageApp := cf.Cf("restage", appName)
				Eventually(restageApp, CF_PUSH_TIMEOUT).Should(Exit(0), "failed restaging app")

				checkForEvents(appName, []string{"audit.app.restage"})

				appEnv := cf.Cf("env", appName).Wait(DEFAULT_TIMEOUT)
				Eventually(appEnv, DEFAULT_TIMEOUT).Should(Exit(0), "failed get env for app")
				Eventually(appEnv.Out.Contents(), DEFAULT_TIMEOUT).Should(ContainSubstring(fmt.Sprintf("credentials")),
					"failed to get credentials for app")
			})

			Context("app is bound to a service", func() {

				BeforeEach(func() {
					bindService := cf.Cf("bind-service", appName, instanceName)
					Eventually(bindService, DEFAULT_TIMEOUT).Should(Exit(0), "failed binding app to service")
				})

				AfterEach(func() {
					deleteApp := cf.Cf("delete", appName, "-f")
					Eventually(deleteApp, DEFAULT_TIMEOUT).Should(Exit(0), "deletion of app failed")

				})

				It("can unbind service from app", func() {
					unbindService := cf.Cf("unbind-service", appName, instanceName)
					Eventually(unbindService, DEFAULT_TIMEOUT).Should(Exit(0), "failed unbinding app to service")

					checkForEvents(appName, []string{"audit.app.update"})

					appEnv := cf.Cf("env", appName)
					Eventually(appEnv, DEFAULT_TIMEOUT).Should(Exit(0), "failed get env for app")
					Eventually(appEnv.Out.Contents(), DEFAULT_TIMEOUT).ShouldNot(ContainSubstring(fmt.Sprintf("credentials")),
						"credentials should not have been found")
				})

			})
		})
	})

	Context("Async broker", func() {
		BeforeEach(func() {
			broker = NewServiceBroker(generator.RandomName(), assets.NewAssets().ServiceBroker, context)
			broker.Push()
			broker.Configure()
			broker.Create()
			broker.PublicizePlans()
		})

		AfterEach(func() {
			broker.Destroy()
		})

		It("can create a service instance", func() {
			instanceName := generator.RandomName()
			createService := cf.Cf("create-service", broker.Service.Name, broker.AsyncPlans[0].Name, instanceName).Wait(DEFAULT_TIMEOUT)
			Expect(createService).To(Exit(0))
			Expect(createService.Out.Contents()).To(ContainSubstring("Create in progress."))

			waitForAsyncOperationToComplete(broker, instanceName)

			serviceInfo := cf.Cf("service", instanceName).Wait(DEFAULT_TIMEOUT)
			Expect(serviceInfo.Out.Contents()).To(ContainSubstring(fmt.Sprintf("Plan: %s", broker.AsyncPlans[0].Name)))
			Expect(serviceInfo.Out.Contents()).To(ContainSubstring("Status: create succeeded"))
			Expect(serviceInfo.Out.Contents()).To(ContainSubstring("Message: 100 percent done"))
		})

		Context("service already exists", func() {
			var instanceName string

			BeforeEach(func() {
				instanceName = generator.RandomName()
				createService := cf.Cf("create-service", broker.Service.Name, broker.AsyncPlans[0].Name, instanceName)
				Eventually(createService, DEFAULT_TIMEOUT).Should(Exit(0), "create service did not exit")
				Eventually(createService.Out.Contents(), DEFAULT_TIMEOUT).Should(ContainSubstring("Create in progress."), "create in progress message not found")

				waitForAsyncOperationToComplete(broker, instanceName)
			})
			It("can update a service instance", func() {
				updateService := cf.Cf("update-service", instanceName, "-p", broker.AsyncPlans[1].Name)
				Eventually(updateService, DEFAULT_TIMEOUT).Should(Exit(0), "update plan command did not exit")
				Eventually(updateService.Out.Contents(), DEFAULT_TIMEOUT).Should(ContainSubstring("Update in progress."), "update in progress message not found")

				waitForAsyncOperationToComplete(broker, instanceName)

				serviceInfo := cf.Cf("service", instanceName).Wait(DEFAULT_TIMEOUT)
				Eventually(serviceInfo, DEFAULT_TIMEOUT).Should(Exit(0), "failed getting service instance details")
				Eventually(serviceInfo.Out.Contents(), DEFAULT_TIMEOUT).Should(ContainSubstring(fmt.Sprintf("Plan: %s", broker.AsyncPlans[1].Name)), "plan not found")

			})
			Context("app already exists", func() {
				var appName string

				BeforeEach(func() {
					appName = generator.RandomName()
					createApp := cf.Cf("push", appName, "-p", assets.NewAssets().Dora)
					Eventually(createApp, CF_PUSH_TIMEOUT).Should(Exit(0), "failed creating app")
				})

				It("can bind a service instance to an app", func() {
					bindService := cf.Cf("bind-service", appName, instanceName)
					Eventually(bindService, DEFAULT_TIMEOUT).Should(Exit(0), "failed binding app to service")

					checkForEvents(appName, []string{"audit.app.update"})

					restageApp := cf.Cf("restage", appName)
					Eventually(restageApp, CF_PUSH_TIMEOUT).Should(Exit(0), "failed restaging app")

					checkForEvents(appName, []string{"audit.app.restage"})

					appEnv := cf.Cf("env", appName)
					Eventually(appEnv, DEFAULT_TIMEOUT).Should(Exit(0), "failed get env for app")
					Eventually(appEnv.Out.Contents(), DEFAULT_TIMEOUT).Should(ContainSubstring(fmt.Sprintf("credentials")), "could not find bound service instance credentials")
				})

				Context("service instance already bound", func() {
					BeforeEach(func() {
						bindService := cf.Cf("bind-service", appName, instanceName)
						Eventually(bindService, DEFAULT_TIMEOUT).Should(Exit(0), "failed binding app to service")
					})

					It("can unbind a service instance", func() {
						unbindService := cf.Cf("unbind-service", appName, instanceName)
						Eventually(unbindService, DEFAULT_TIMEOUT).Should(Exit(0), "failed unbinding app to service")

						checkForEvents(appName, []string{"audit.app.update"})

						appEnv := cf.Cf("env", appName)
						Eventually(appEnv, DEFAULT_TIMEOUT).Should(Exit(0), "failed get env for app")
						Eventually(appEnv.Out.Contents(), DEFAULT_TIMEOUT).ShouldNot(ContainSubstring(fmt.Sprintf("credentials")), "error: credentials should not have been found")
					})
				})
			})

			It("can delete a service instance", func() {
				deleteService := cf.Cf("delete-service", instanceName, "-f")
				Eventually(deleteService, DEFAULT_TIMEOUT).Should(Exit(0), "failed making delete request")
				Eventually(deleteService.Out.Contents(), DEFAULT_TIMEOUT).Should(ContainSubstring("Delete in progress."), "deletion not in progress")

				waitForAsyncDeletionToComplete(broker, instanceName)
			})
		})
	})
})

func checkForEvents(name string, eventNames []string) {
	events := cf.Cf("events", name).Wait(DEFAULT_TIMEOUT)
	Expect(events).To(Exit(0), fmt.Sprintf("failed getting events for %s", name))

	for _, eventName := range eventNames {
		Expect(events.Out.Contents()).To(ContainSubstring(eventName), "failed to find event")
	}
}
