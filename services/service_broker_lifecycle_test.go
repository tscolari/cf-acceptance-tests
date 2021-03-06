package services_test

import (
	"encoding/json"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gbytes"
	. "github.com/onsi/gomega/gexec"

	"github.com/cloudfoundry-incubator/cf-test-helpers/cf"
	"github.com/cloudfoundry-incubator/cf-test-helpers/generator"
	"github.com/cloudfoundry-incubator/cf-test-helpers/helpers"
	"github.com/cloudfoundry/cf-acceptance-tests/helpers/app_helpers"
	"github.com/cloudfoundry/cf-acceptance-tests/helpers/assets"
	. "github.com/cloudfoundry/cf-acceptance-tests/helpers/services"
)

var _ = Describe("Service Broker Lifecycle", func() {
	var broker ServiceBroker

	Describe("public brokers", func() {
		var acls *Session
		var output []byte
		var oldServiceName string
		var oldPlanName string

		BeforeEach(func() {
			broker = NewServiceBroker(
				generator.PrefixedRandomName("pblc-brkr-"),
				assets.NewAssets().ServiceBroker,
				context,
			)
			cf.TargetSpace(context.RegularUserContext(), context.ShortTimeout())
			broker.Push()
			broker.Configure()

			cf.AsUser(context.AdminUserContext(), context.ShortTimeout(), func() {
				broker.Create()
			})
		})

		Describe("Updating the catalog", func() {

			BeforeEach(func() {
				broker.PublicizePlans()
			})

			It("updates the broker and sees catalog changes", func() {
				// Confirming plans show up in the marketplace for regular user
				plans := cf.Cf("marketplace").Wait(DEFAULT_TIMEOUT)
				Expect(plans).To(Exit(0))
				Expect(plans).To(Say(broker.Service.Name))

				Expect(plans).To(Say(broker.SyncPlans[0].Name))
				Expect(plans).To(Say(broker.SyncPlans[1].Name))

				// Changing the catalog on the broker
				oldServiceName = broker.Service.Name
				oldPlanName = broker.SyncPlans[0].Name
				broker.Service.Name = generator.PrefixedRandomName("pblc-brkr-")
				broker.SyncPlans[0].Name = generator.PrefixedRandomName("pblc-brkr-")
				broker.Configure()
				broker.Update()

				// Confirming the changes to the broker show up in the marketplace
				plans = cf.Cf("marketplace").Wait(DEFAULT_TIMEOUT)
				Expect(plans).To(Exit(0))
				Expect(plans).NotTo(Say(oldServiceName))
				Expect(plans).NotTo(Say(oldPlanName))
				Expect(plans).To(Say(broker.Service.Name))
				Expect(plans).To(Say(broker.Plans()[0].Name))

				// Deleting the service broker and confirming the plans no longer display
				cf.AsUser(context.AdminUserContext(), context.ShortTimeout(), func() {
					broker.Delete()
				})

				plans = cf.Cf("marketplace").Wait(DEFAULT_TIMEOUT)
				Expect(plans).To(Exit(0))
				Expect(plans).NotTo(Say(oldServiceName))
				Expect(plans).NotTo(Say(oldPlanName))
				Expect(plans).NotTo(Say(broker.Service.Name))
				Expect(plans).NotTo(Say(broker.Plans()[0].Name))
			})
		})

		Describe("service access", func() {
			var (
				accessOutput        = "%[1]s\\s{2,}%[2]s\\s{2,}%[3]s\\s*\n"
				accessOutputWithOrg = "%[1]s\\s{2,}%[2]s\\s{2,}%[3]s\\s{2,}%[4]s\\s*"
				globallyPublicPlan  Plan
				orgPublicPlan       Plan
			)

			BeforeEach(func() {
				globallyPublicPlan = broker.Plans()[0]
				orgPublicPlan = broker.Plans()[1]
			})

			Describe("enabling", func() {
				It("is visible to a regular user", func() {
					cf.AsUser(context.AdminUserContext(), context.ShortTimeout(), func() {
						commandResult := cf.Cf("enable-service-access", broker.Service.Name, "-p", globallyPublicPlan.Name).Wait(DEFAULT_TIMEOUT)
						Expect(commandResult).To(Exit(0))
						commandResult = cf.Cf("enable-service-access", broker.Service.Name, "-p", orgPublicPlan.Name, "-o", context.RegularUserContext().Org).Wait(DEFAULT_TIMEOUT)
						Expect(commandResult).To(Exit(0))
					})

					plans := cf.Cf("marketplace").Wait(DEFAULT_TIMEOUT)
					Expect(plans).To(Exit(0))
					Expect(plans).To(Say(broker.Service.Name))

					Expect(plans).To(Say(globallyPublicPlan.Name))
					Expect(plans).To(Say(orgPublicPlan.Name))
				})

				It("is visible to an admin user", func() {
					cf.AsUser(context.AdminUserContext(), context.ShortTimeout(), func() {
						commandResult := cf.Cf("enable-service-access", broker.Service.Name, "-p", globallyPublicPlan.Name).Wait(DEFAULT_TIMEOUT)
						Expect(commandResult).To(Exit(0))
						commandResult = cf.Cf("enable-service-access", broker.Service.Name, "-p", orgPublicPlan.Name, "-o", context.RegularUserContext().Org).Wait(DEFAULT_TIMEOUT)
						Expect(commandResult).To(Exit(0))

						acls = cf.Cf("service-access", "-e", broker.Service.Name).Wait(DEFAULT_TIMEOUT)
						Expect(acls).To(Exit(0))
						output = acls.Out.Contents()
						Expect(output).To(ContainSubstring(broker.Service.Name))

						expectedOutput := fmt.Sprintf(accessOutput, broker.Service.Name, globallyPublicPlan.Name, "all")
						Expect(output).To(MatchRegexp(expectedOutput))

						expectedOutput = fmt.Sprintf(accessOutputWithOrg, broker.Service.Name, orgPublicPlan.Name, "limited", context.RegularUserContext().Org)
						Expect(output).To(MatchRegexp(expectedOutput))
					})
				})
			})

			Describe("disabling", func() {

				BeforeEach(func() {
					cf.AsUser(context.AdminUserContext(), context.ShortTimeout(), func() {
						commandResult := cf.Cf("enable-service-access", broker.Service.Name, "-p", globallyPublicPlan.Name).Wait(DEFAULT_TIMEOUT)
						Expect(commandResult).To(Exit(0))
						commandResult = cf.Cf("enable-service-access", broker.Service.Name, "-p", orgPublicPlan.Name, "-o", context.RegularUserContext().Org).Wait(DEFAULT_TIMEOUT)
						Expect(commandResult).To(Exit(0))
					})
				})

				It("is not visible to a regular user", func() {
					cf.AsUser(context.AdminUserContext(), context.ShortTimeout(), func() {
						commandResult := cf.Cf("disable-service-access", broker.Service.Name, "-p", orgPublicPlan.Name, "-o", context.RegularUserContext().Org).Wait(DEFAULT_TIMEOUT)
						Expect(commandResult).To(Exit(0))
						commandResult = cf.Cf("disable-service-access", broker.Service.Name, "-p", globallyPublicPlan.Name).Wait(DEFAULT_TIMEOUT)
						Expect(commandResult).To(Exit(0))
					})

					plans := cf.Cf("marketplace").Wait(DEFAULT_TIMEOUT)
					Expect(plans).To(Exit(0))
					Expect(plans).NotTo(Say(broker.Service.Name))

					Expect(plans).NotTo(Say(globallyPublicPlan.Name))
					Expect(plans).NotTo(Say(orgPublicPlan.Name))
				})

				It("is visible as having no access to an admin user", func() {
					cf.AsUser(context.AdminUserContext(), context.ShortTimeout(), func() {
						commandResult := cf.Cf("disable-service-access", broker.Service.Name, "-p", orgPublicPlan.Name, "-o", context.RegularUserContext().Org).Wait(DEFAULT_TIMEOUT)
						Expect(commandResult).To(Exit(0))
						commandResult = cf.Cf("disable-service-access", broker.Service.Name, "-p", globallyPublicPlan.Name).Wait(DEFAULT_TIMEOUT)
						Expect(commandResult).To(Exit(0))
						acls = cf.Cf("service-access", "-e", broker.Service.Name).Wait(DEFAULT_TIMEOUT)
						Expect(acls).To(Exit(0))
						output = acls.Out.Contents()
						Expect(output).To(ContainSubstring(broker.Service.Name))

						expectedOutput := fmt.Sprintf(accessOutput, broker.Service.Name, globallyPublicPlan.Name, "none")
						Expect(output).To(MatchRegexp(expectedOutput))
						expectedOutput = fmt.Sprintf(accessOutput, broker.Service.Name, orgPublicPlan.Name, "none")
						Expect(output).To(MatchRegexp(expectedOutput))
					})
				})
			})
		})

		AfterEach(func() {
			app_helpers.AppReport(broker.Name, DEFAULT_TIMEOUT)

			broker.Destroy()
		})
	})

	Describe("private brokers", func() {
		BeforeEach(func() {
			broker = NewServiceBroker(
				generator.PrefixedRandomName("prbr-"),
				assets.NewAssets().ServiceBroker,
				context,
			)
			cf.TargetSpace(context.RegularUserContext(), context.ShortTimeout())
			broker.Push()
			broker.Configure()
		})

		AfterEach(func() {
			broker.Destroy()
		})

		It("can be created, viewed (in list), updated, and deleted by SpaceDevelopers", func() {
			cf.AsUser(context.RegularUserContext(), context.ShortTimeout(), func() {
				spaceCmd := cf.Cf("space", context.RegularUserContext().Space, "--guid").Wait(DEFAULT_TIMEOUT)
				spaceGuid := string(spaceCmd.Out.Contents())
				spaceGuid = strings.Trim(spaceGuid, "\n")
				body := map[string]string{
					"name":          broker.Name,
					"broker_url":    helpers.AppUri(broker.Name, ""),
					"auth_username": context.RegularUserContext().Username,
					"auth_password": context.RegularUserContext().Password,
					"space_guid":    spaceGuid,
				}
				jsonBody, _ := json.Marshal(body)

				By("Create")
				createBrokerCommand := cf.Cf("curl", "/v2/service_brokers", "-X", "POST", "-d", string(jsonBody)).Wait(DEFAULT_TIMEOUT)
				Expect(createBrokerCommand).To(Exit(0))

				By("Read")
				serviceBrokersCommand := cf.Cf("service-brokers").Wait(DEFAULT_TIMEOUT)
				Expect(serviceBrokersCommand).To(Exit(0))
				Expect(serviceBrokersCommand.Out.Contents()).To(ContainSubstring(broker.Name))

				By("Update")
				updatedName := broker.Name + "updated"
				updateBrokerCommand := cf.Cf("rename-service-broker", broker.Name, updatedName).Wait(DEFAULT_TIMEOUT)
				Expect(updateBrokerCommand).To(Exit(0))

				serviceBrokersCommand = cf.Cf("service-brokers").Wait(DEFAULT_TIMEOUT)
				Expect(serviceBrokersCommand).To(Exit(0))
				Expect(serviceBrokersCommand.Out.Contents()).To(ContainSubstring(updatedName))

				By("Delete")
				deleteBrokerCommand := cf.Cf("delete-service-broker", updatedName, "-f").Wait(DEFAULT_TIMEOUT)
				Expect(deleteBrokerCommand).To(Exit(0))

				serviceBrokersCommand = cf.Cf("service-brokers").Wait(DEFAULT_TIMEOUT)
				Expect(serviceBrokersCommand).To(Exit(0))
				Expect(serviceBrokersCommand.Out.Contents()).NotTo(ContainSubstring(updatedName))
			})
		})

		It("exposes the services and plans of the private broker in the space", func() {
			cf.AsUser(context.RegularUserContext(), context.ShortTimeout(), func() {
				spaceCmd := cf.Cf("space", context.RegularUserContext().Space, "--guid").Wait(DEFAULT_TIMEOUT)
				spaceGuid := string(spaceCmd.Out.Contents())
				spaceGuid = strings.Trim(spaceGuid, "\n")
				body := map[string]string{
					"name":          broker.Name,
					"broker_url":    helpers.AppUri(broker.Name, ""),
					"auth_username": context.RegularUserContext().Username,
					"auth_password": context.RegularUserContext().Password,
					"space_guid":    spaceGuid,
				}
				jsonBody, _ := json.Marshal(body)

				createBrokerCommand := cf.Cf("curl", "/v2/service_brokers", "-X", "POST", "-d", string(jsonBody)).Wait(DEFAULT_TIMEOUT)
				Expect(createBrokerCommand).To(Exit(0))

				marketplaceOutput := cf.Cf("marketplace").Wait(DEFAULT_TIMEOUT)

				Expect(marketplaceOutput).To(Exit(0))
				Expect(marketplaceOutput).To(Say(broker.Service.Name))
				Expect(marketplaceOutput).To(Say(broker.SyncPlans[0].Name))
				Expect(marketplaceOutput).To(Say(broker.SyncPlans[1].Name))
			})
		})
	})
})
