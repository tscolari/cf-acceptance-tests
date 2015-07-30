package services_test

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/cloudfoundry-incubator/cf-test-helpers/cf"
	"github.com/cloudfoundry-incubator/cf-test-helpers/generator"
	"github.com/cloudfoundry/cf-acceptance-tests/helpers/assets"
	. "github.com/cloudfoundry/cf-acceptance-tests/helpers/services"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gbytes"
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

type Params struct{ Param1 string }

var _ = Describe("Service Instance Lifecycle", func() {
	var broker ServiceBroker
	var ASYNC_OPERATION_TIMEOUT = 2 * time.Minute
	var ASYNC_OPERATION_POLL_INTERVAL = 5 * time.Second

	waitForAsyncDeletionToComplete := func(broker ServiceBroker, instanceName string) {
		Eventually(func() *Session {
			return cf.Cf("service", instanceName).Wait(DEFAULT_TIMEOUT)
		}, ASYNC_OPERATION_TIMEOUT, ASYNC_OPERATION_POLL_INTERVAL).Should(Say("not found"))
	}

	waitForAsyncOperationToComplete := func(broker ServiceBroker, instanceName string) {
		Eventually(func() *Session {
			serviceDetails := cf.Cf("service", instanceName).Wait(DEFAULT_TIMEOUT)
			Expect(serviceDetails).To(Exit(0), "failed getting service instance details")
			return serviceDetails
		}, ASYNC_OPERATION_TIMEOUT, ASYNC_OPERATION_POLL_INTERVAL).Should(Say("succeeded"))
	}

	Context("Synchronous operations", func() {
		BeforeEach(func() {
			broker = NewServiceBroker(generator.RandomName(), assets.NewAssets().ServiceBroker, context, true)
			broker.Push()
			broker.Configure()
			broker.Create()
			broker.EnableServiceAccess()
		})

		AfterEach(func() {
			broker.Destroy()
		})

		Context("just service instances", func() {
			It("can create, delete and update a service instance", func() {
				By("creating a service instance")
				// CREATE

				tags := "['tag1', 'tag2']"
				params, _ := json.Marshal(Params{Param1: "value"})

				instanceName := generator.RandomName()
				createService := cf.Cf("create-service", broker.Service.Name, broker.SyncPlans[0].Name, instanceName, "-c", string(params), "-t", tags).Wait(DEFAULT_TIMEOUT)
				Expect(createService).To(Exit(0))

				os.Setenv("CF_TRACE", "true")
				serviceInfo := cf.Cf("service", instanceName).Wait(DEFAULT_TIMEOUT)
				Expect(serviceInfo).To(Say(fmt.Sprintf("Plan: %s", broker.SyncPlans[0].Name)))
				Expect(serviceInfo.Out.Contents()).To(MatchRegexp(`"tags":\s*\[\n.*tag1.*\n.*tag2.*\n.*\]`))
				os.Setenv("CF_TRACE", "false")

				// UPDATE

				tags = "['tag1', 'tag2']"
				params, _ = json.Marshal(Params{Param1: "value"})

				By("renaming a service")

				newInstanceName := generator.RandomName()
				updateService := cf.Cf("rename-service", instanceName, newInstanceName).Wait(DEFAULT_TIMEOUT)
				Expect(updateService).To(Exit(0))

				serviceInfo = cf.Cf("service", newInstanceName).Wait(DEFAULT_TIMEOUT)
				Expect(serviceInfo).To(Say(newInstanceName))

				serviceInfo = cf.Cf("service", instanceName).Wait(DEFAULT_TIMEOUT)
				Expect(serviceInfo).To(Exit(1))

				By("updating a service plan")

				updateService = cf.Cf("update-service", newInstanceName, "-p", broker.SyncPlans[1].Name).Wait(DEFAULT_TIMEOUT)
				Expect(updateService).To(Exit(0))

				serviceInfo = cf.Cf("service", newInstanceName).Wait(DEFAULT_TIMEOUT)
				Expect(serviceInfo).To(Say(fmt.Sprintf("Plan: %s", broker.SyncPlans[1].Name)))

				By("updating service tags")

				updateService = cf.Cf("update-service", newInstanceName, "-t", tags).Wait(DEFAULT_TIMEOUT)
				Expect(updateService).To(Exit(0))

				os.Setenv("CF_TRACE", "true")
				serviceInfo = cf.Cf("service", newInstanceName).Wait(DEFAULT_TIMEOUT)
				Expect(serviceInfo.Out.Contents()).To(MatchRegexp(`"tags":\s*\[\n.*tag1.*\n.*tag2.*\n.*\]`))
				os.Setenv("CF_TRACE", "false")

				By("updating arbitrary parameters")

				updateService = cf.Cf("update-service", newInstanceName, "-c", string(params)).Wait(DEFAULT_TIMEOUT)
				Expect(updateService).To(Exit(0))
				//Note: We don't necessarily get these back through a service instance lookup, they are only passed to the broker

				By("updating all available parameters at once")

				updateService = cf.Cf(
					"update-service", newInstanceName,
					"-p", broker.SyncPlans[1].Name,
					"-t", tags,
					"-c", string(params)).Wait(DEFAULT_TIMEOUT)
				Expect(updateService).To(Exit(0))

				os.Setenv("CF_TRACE", "true")
				serviceInfo = cf.Cf("service", newInstanceName).Wait(DEFAULT_TIMEOUT)
				Expect(serviceInfo).To(Say(fmt.Sprintf("Plan: %s", broker.SyncPlans[1].Name)))
				Expect(serviceInfo.Out.Contents()).To(MatchRegexp(`"tags":\s*\[\n.*tag1.*\n.*tag2.*\n.*\]`))
				os.Setenv("CF_TRACE", "false")

				// DELETE
				By("deleting a service instance")

				deleteService := cf.Cf("delete-service", newInstanceName, "-f").Wait(DEFAULT_TIMEOUT)
				Expect(deleteService).To(Exit(0))

				serviceInfo = cf.Cf("service", newInstanceName).Wait(DEFAULT_TIMEOUT)
				Expect(serviceInfo).To(Say("not found"))
			})

			Context("when there is an existing service instance", func() {
				var instanceName string

				BeforeEach(func() {
					instanceName = generator.RandomName()
					createService := cf.Cf("create-service", broker.Service.Name, broker.SyncPlans[0].Name, instanceName).Wait(DEFAULT_TIMEOUT)
					Expect(createService).To(Exit(0), "failed creating service")
				})

				Describe("service keys", func() {
					It("can create and delete service keys", func() {
						By("creating a service key")

						keyName := generator.RandomName()

						createKey := cf.Cf("create-service-key", instanceName, keyName).Wait(DEFAULT_TIMEOUT)
						Expect(createKey).To(Exit(0))

						keyInfo := cf.Cf("service-key", instanceName, keyName).Wait(DEFAULT_TIMEOUT)
						Expect(keyInfo).To(Exit(0), "failed key info")

						Expect(keyInfo).To(Say(`"database": "fake-dbname"`))
						Expect(keyInfo).To(Say(`"password": "fake-password"`))
						Expect(keyInfo).To(Say(`"username": "fake-user"`))

						By("creating a service key with arbitrary params")

						keyWithArbitraryParams := generator.RandomName()
						params, _ := json.Marshal(Params{Param1: "value"})

						createKey = cf.Cf("create-service-key", instanceName, keyWithArbitraryParams, "-c", string(params)).Wait(DEFAULT_TIMEOUT)
						Expect(createKey).To(Exit(0))

						keyInfo = cf.Cf("service-key", instanceName, keyWithArbitraryParams).Wait(DEFAULT_TIMEOUT)
						Expect(keyInfo).To(Exit(0), "failed key info")

						By("deleting the service key")

						deleteServiceKey := cf.Cf("delete-service-key", instanceName, keyName, "-f").Wait(DEFAULT_TIMEOUT)
						Expect(deleteServiceKey).To(Exit(0))

						keyInfo = cf.Cf("service-key", instanceName, keyName).Wait(DEFAULT_TIMEOUT)
						Expect(keyInfo).To(Exit(0), "failed key info")

						errorMessage := fmt.Sprintf("No service key %s found for service instance %s", keyName, instanceName)
						Expect(keyInfo).To(Say(errorMessage))
					})
				})
			})
		})

		Context("when there is an app", func() {
			var instanceName, appName string

			BeforeEach(func() {
				appName = generator.PrefixedRandomName("CATS-APP-")
				createApp := cf.Cf("push", appName, "-p", assets.NewAssets().Dora).Wait(CF_PUSH_TIMEOUT)
				Expect(createApp).To(Exit(0), "failed creating app")

				checkForEvents(appName, []string{"audit.app.create"})

				instanceName = generator.RandomName()
				createService := cf.Cf("create-service", broker.Service.Name, broker.SyncPlans[0].Name, instanceName).Wait(DEFAULT_TIMEOUT)
				Expect(createService).To(Exit(0), "failed creating service")
			})
			Describe("bindings", func() {
				It("can bind service to app and unbind the service", func() {
					bindService := cf.Cf("bind-service", appName, instanceName).Wait(DEFAULT_TIMEOUT)
					Expect(bindService).To(Exit(0), "failed binding app to service")

					checkForEvents(appName, []string{"audit.app.update"})

					restageApp := cf.Cf("restage", appName).Wait(CF_PUSH_TIMEOUT)
					Expect(restageApp).To(Exit(0), "failed restaging app")

					checkForEvents(appName, []string{"audit.app.restage"})

					appEnv := cf.Cf("env", appName).Wait(DEFAULT_TIMEOUT)
					Expect(appEnv).To(Exit(0), "failed get env for app")
					Expect(appEnv).To(Say(fmt.Sprintf("credentials")))

					unbindService := cf.Cf("unbind-service", appName, instanceName).Wait(DEFAULT_TIMEOUT)
					Expect(unbindService).To(Exit(0), "failed unbinding app to service")

					checkForEvents(appName, []string{"audit.app.update"})

					appEnv = cf.Cf("env", appName).Wait(DEFAULT_TIMEOUT)
					Expect(appEnv).To(Exit(0), "failed get env for app")
					Expect(appEnv).ToNot(Say(fmt.Sprintf("credentials")))
				})
			})

		})
	})

	Context("Asynchronous operations", func() {
		BeforeEach(func() {
			broker = NewServiceBroker(generator.RandomName(), assets.NewAssets().ServiceBroker, context, true)
			broker.Push()
			broker.Configure()
			broker.Create()
			broker.EnableServiceAccess()
		})

		AfterEach(func() {
			broker.Destroy()
		})

		It("can create, update and delete a service instance", func() {
			tags := "['tag1', 'tag2']"
			type Params struct{ Param1 string }
			params, _ := json.Marshal(Params{Param1: "value"})

			By("creating a service instance")

			instanceName := generator.RandomName()
			createService := cf.Cf("create-service", broker.Service.Name, broker.AsyncPlans[0].Name, instanceName, "-t", tags, "-c", string(params)).Wait(DEFAULT_TIMEOUT)
			Expect(createService).To(Exit(0))
			Expect(createService).To(Say("Create in progress."))

			waitForAsyncOperationToComplete(broker, instanceName)

			os.Setenv("CF_TRACE", "true")

			serviceInfo := cf.Cf("service", instanceName).Wait(DEFAULT_TIMEOUT)

			Expect(serviceInfo).To(Say(fmt.Sprintf("Plan: %s", broker.AsyncPlans[0].Name)))
			Expect(serviceInfo).To(Say("Status: create succeeded"))
			Expect(serviceInfo).To(Say("Message: 100 percent done"))
			Expect(serviceInfo.Out.Contents()).To(MatchRegexp(`"tags":\s*\[\n.*tag1.*\n.*tag2.*\n.*\]`))

			os.Setenv("CF_TRACE", "false")

			By("updating a service plan")

			updateService := cf.Cf("update-service", instanceName, "-p", broker.AsyncPlans[1].Name).Wait(DEFAULT_TIMEOUT)

			Expect(updateService).To(Exit(0))
			Expect(updateService).To(Say("Update in progress."))

			serviceInfo = cf.Cf("service", instanceName).Wait(DEFAULT_TIMEOUT)

			Expect(serviceInfo).To(Exit(0), "failed getting service instance details")
			Expect(serviceInfo).To(Say(fmt.Sprintf("Plan: %s", broker.AsyncPlans[0].Name)))

			waitForAsyncOperationToComplete(broker, instanceName)

			serviceInfo = cf.Cf("service", instanceName).Wait(DEFAULT_TIMEOUT)
			Expect(serviceInfo).To(Exit(0), "failed getting service instance details")
			Expect(serviceInfo).To(Say(fmt.Sprintf("Plan: %s", broker.AsyncPlans[1].Name)))

			By("updating the arbitrary params")

			updatedParams, _ := json.Marshal(Params{Param1: "newValue"})
			updateService = cf.Cf("update-service", instanceName, "-c", string(updatedParams)).Wait(DEFAULT_TIMEOUT)
			Expect(updateService).To(Exit(0))
			Expect(updateService).To(Say("Update in progress."))

			waitForAsyncOperationToComplete(broker, instanceName)

			By("updating all of the available parameters at once")

			moreUpdatedParams, _ := json.Marshal(Params{Param1: "newerValue"})

			updateService = cf.Cf(
				"update-service", instanceName,
				"-t", tags,
				"-c", string(moreUpdatedParams),
				"-p", broker.AsyncPlans[1].Name).Wait(DEFAULT_TIMEOUT)

			Expect(updateService).To(Exit(0))
			Expect(updateService).To(Say("Update in progress."))

			waitForAsyncOperationToComplete(broker, instanceName)

			os.Setenv("CF_TRACE", "true")
			serviceInfo = cf.Cf("service", instanceName).Wait(DEFAULT_TIMEOUT)

			Expect(serviceInfo).To(Exit(0), "failed getting service instance details")
			Expect(serviceInfo).To(Say(fmt.Sprintf("Plan: %s", broker.AsyncPlans[1].Name)))
			Expect(serviceInfo.Out.Contents()).To(MatchRegexp(`"tags":\s*\[\n.*tag1.*\n.*tag2.*\n.*\]`))

			os.Setenv("CF_TRACE", "false")

			By("deleting a service instance")

			deleteService := cf.Cf("delete-service", instanceName, "-f").Wait(DEFAULT_TIMEOUT)
			Expect(deleteService).To(Exit(0), "failed making delete request")
			Expect(deleteService).To(Say("Delete in progress."))

			waitForAsyncDeletionToComplete(broker, instanceName)
		})

		Context("when there is an existing service instance", func() {
			var instanceName string
			BeforeEach(func() {
				instanceName = generator.RandomName()
				createService := cf.Cf("create-service", broker.Service.Name, broker.AsyncPlans[0].Name, instanceName).Wait(DEFAULT_TIMEOUT)
				Expect(createService).To(Exit(0))
				Expect(createService).To(Say("Create in progress."))

				waitForAsyncOperationToComplete(broker, instanceName)
			})

			Context("when there is an app", func() {
				var appName string
				BeforeEach(func() {
					appName = generator.PrefixedRandomName("CATS-APP-")
					createApp := cf.Cf("push", appName, "-p", assets.NewAssets().Dora).Wait(CF_PUSH_TIMEOUT)
					Expect(createApp).To(Exit(0), "failed creating app")
				})

				It("can bind and unbind a service instance", func() {
					bindService := cf.Cf("bind-service", appName, instanceName).Wait(DEFAULT_TIMEOUT)
					Expect(bindService).To(Exit(0), "failed binding app to service")

					checkForEvents(appName, []string{"audit.app.update"})

					restageApp := cf.Cf("restage", appName).Wait(CF_PUSH_TIMEOUT)
					Expect(restageApp).To(Exit(0), "failed restaging app")

					checkForEvents(appName, []string{"audit.app.restage"})

					appEnv := cf.Cf("env", appName).Wait(DEFAULT_TIMEOUT)
					Expect(appEnv).To(Exit(0), "failed get env for app")
					Expect(appEnv).To(Say(fmt.Sprintf("credentials")))

					unbindService := cf.Cf("unbind-service", appName, instanceName).Wait(DEFAULT_TIMEOUT)
					Expect(unbindService).To(Exit(0), "failed unbinding app to service")

					checkForEvents(appName, []string{"audit.app.update"})

					appEnv = cf.Cf("env", appName).Wait(DEFAULT_TIMEOUT)
					Expect(appEnv).To(Exit(0), "failed get env for app")
					Expect(appEnv).ToNot(Say(fmt.Sprintf("credentials")))
				})
			})
		})
	})
})

func checkForEvents(name string, eventNames []string) {
	events := cf.Cf("events", name).Wait(DEFAULT_TIMEOUT)
	Expect(events).To(Exit(0), fmt.Sprintf("failed getting events for %s", name))

	for _, eventName := range eventNames {
		Expect(events).To(Say(eventName), "failed to find event")
	}
}
