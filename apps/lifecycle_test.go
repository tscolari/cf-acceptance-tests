package apps

import (
	"encoding/json"
	"fmt"
	"strings"

	. "github.com/cloudfoundry/cf-acceptance-tests/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "github.com/cloudfoundry/cf-acceptance-tests/Godeps/_workspace/src/github.com/onsi/gomega"
	. "github.com/cloudfoundry/cf-acceptance-tests/Godeps/_workspace/src/github.com/onsi/gomega/gbytes"
	. "github.com/cloudfoundry/cf-acceptance-tests/Godeps/_workspace/src/github.com/onsi/gomega/gexec"

	"github.com/cloudfoundry/cf-acceptance-tests/Godeps/_workspace/src/github.com/cloudfoundry-incubator/cf-test-helpers/cf"
	"github.com/cloudfoundry/cf-acceptance-tests/Godeps/_workspace/src/github.com/cloudfoundry-incubator/cf-test-helpers/generator"
	"github.com/cloudfoundry/cf-acceptance-tests/Godeps/_workspace/src/github.com/cloudfoundry-incubator/cf-test-helpers/helpers"
	"github.com/cloudfoundry/cf-acceptance-tests/helpers/app_helpers"
	"github.com/cloudfoundry/cf-acceptance-tests/helpers/assets"
)

type AppUsageEvent struct {
	Entity struct {
		AppName       string `json:"app_name"`
		State         string `json:"state"`
		BuildpackName string `json:"buildpack_name"`
		BuildpackGuid string `json:"buildpack_guid"`
	} `json:"entity"`
}

type AppUsageEvents struct {
	Resources []AppUsageEvent `struct:"resources"`
}

func lastAppUsageEvent(appName string, state string) (bool, AppUsageEvent) {
	var response AppUsageEvents
	cf.AsUser(context.AdminUserContext(), DEFAULT_TIMEOUT, func() {
		cf.ApiRequest("GET", "/v2/app_usage_events?order-direction=desc&page=1&results-per-page=150", &response, DEFAULT_TIMEOUT)
	})

	for _, event := range response.Resources {
		if event.Entity.AppName == appName && event.Entity.State == state {
			return true, event
		}
	}

	return false, AppUsageEvent{}
}

var _ = Describe("Application Lifecycle", func() {
	var appName string

	BeforeEach(func() {
		appName = generator.PrefixedRandomName("CATS-APP-")
	})

	AfterEach(func() {
		app_helpers.AppReport(appName, DEFAULT_TIMEOUT)

		Expect(cf.Cf("delete", appName, "-f", "-r").Wait(DEFAULT_TIMEOUT)).To(Exit(0))
	})

	Describe("pushing", func() {
		It("makes the app reachable via its bound route", func() {
			Expect(cf.Cf("push", appName, "--no-start", "-b", config.RubyBuildpackName, "-m", DEFAULT_MEMORY_LIMIT, "-p", assets.NewAssets().Dora, "-d", config.AppsDomain).Wait(DEFAULT_TIMEOUT)).To(Exit(0))
			app_helpers.SetBackend(appName)

			Expect(cf.Cf("start", appName).Wait(CF_PUSH_TIMEOUT)).To(Exit(0))

			Eventually(func() string {
				return helpers.CurlAppRoot(appName)
			}, DEFAULT_TIMEOUT).Should(ContainSubstring("Hi, I'm Dora!"))
		})

		Describe("Context path", func() {
			var app2 string
			var path = "/imposter_dora"

			BeforeEach(func() {
				Expect(cf.Cf("push", appName, "--no-start", "-b", config.RubyBuildpackName, "-m", DEFAULT_MEMORY_LIMIT, "-p", assets.NewAssets().Dora, "-d", config.AppsDomain).Wait(DEFAULT_TIMEOUT)).To(Exit(0))
				app_helpers.SetBackend(appName)

				Expect(cf.Cf("start", appName).Wait(CF_PUSH_TIMEOUT)).To(Exit(0))

				app2 = generator.PrefixedRandomName("CATS-APP-")
				Expect(cf.Cf("push", app2, "--no-start", "-b", config.RubyBuildpackName, "-m", DEFAULT_MEMORY_LIMIT, "-p", assets.NewAssets().HelloWorld, "-d", config.AppsDomain).Wait(DEFAULT_TIMEOUT)).To(Exit(0))
				app_helpers.SetBackend(app2)
				Expect(cf.Cf("start", app2).Wait(CF_PUSH_TIMEOUT)).To(Exit(0))
			})

			AfterEach(func() {
				Expect(cf.Cf("delete", app2, "-f", "-r").Wait(DEFAULT_TIMEOUT)).To(Exit(0))
			})

			It("makes another app available via same host and domain, but different path", func() {
				getRoutePath := fmt.Sprintf("/v2/routes?q=host:%s", appName)
				routeBody := cf.Cf("curl", getRoutePath).Wait(DEFAULT_TIMEOUT).Out.Contents()
				var routeJSON struct {
					Resources []struct {
						Entity struct {
							SpaceGuid  string `json:"space_guid"`
							DomainGuid string `json:"domain_guid"`
						} `json:"entity"`
					} `json:"resources"`
				}
				json.Unmarshal([]byte(routeBody), &routeJSON)

				spaceGuid := routeJSON.Resources[0].Entity.SpaceGuid
				domainGuid := routeJSON.Resources[0].Entity.DomainGuid
				appGuid := cf.Cf("app", app2, "--guid").Wait(DEFAULT_TIMEOUT).Out.Contents()

				jsonBody := "{\"host\":\"" + appName + "\", \"path\":\"" + path + "\", \"domain_guid\":\"" + domainGuid + "\",\"space_guid\":\"" + spaceGuid + "\"}"
				routePostResponseBody := cf.Cf("curl", "/v2/routes", "-X", "POST", "-d", jsonBody).Wait(CF_PUSH_TIMEOUT).Out.Contents()

				var routeResponseJSON struct {
					Metadata struct {
						Guid string `json:"guid"`
					} `json:"metadata"`
				}
				json.Unmarshal([]byte(routePostResponseBody), &routeResponseJSON)
				routeGuid := routeResponseJSON.Metadata.Guid

				Expect(cf.Cf("curl", "/v2/apps/"+strings.TrimSpace(string(appGuid))+"/routes/"+string(routeGuid), "-X", "PUT").Wait(CF_PUSH_TIMEOUT)).To(Exit(0))

				Eventually(func() string {
					return helpers.CurlAppRoot(appName)
				}, DEFAULT_TIMEOUT).Should(ContainSubstring("Hi, I'm Dora!"))

				Eventually(func() string {
					return helpers.CurlApp(appName, path)
				}, DEFAULT_TIMEOUT).Should(ContainSubstring("Hello, world!"))
			})
		})

		Context("multiple instances", func() {
			BeforeEach(func() {
				Expect(cf.Cf("push", appName, "--no-start", "-b", config.RubyBuildpackName, "-m", DEFAULT_MEMORY_LIMIT, "-p", assets.NewAssets().Dora, "-d", config.AppsDomain).Wait(DEFAULT_TIMEOUT)).To(Exit(0))
				app_helpers.SetBackend(appName)
				Expect(cf.Cf("scale", appName, "-i", "2").Wait(DEFAULT_TIMEOUT)).To(Exit(0))
			})

			It("is able to start all instances", func() {
				Expect(cf.Cf("start", appName).Wait(CF_PUSH_TIMEOUT)).To(Exit(0))

				retryTimeout := 3 * DEFAULT_TIMEOUT

				Eventually(func() *Session {
					return cf.Cf("app", appName).Wait(DEFAULT_TIMEOUT)
				}, retryTimeout).Should(Say("#0   running"))

				Eventually(func() *Session {
					return cf.Cf("app", appName).Wait(DEFAULT_TIMEOUT)
				}, retryTimeout).Should(Say("#1   running"))
			})
		})

		It("makes system environment variables available", func() {
			Expect(cf.Cf("push", appName, "--no-start", "-b", config.RubyBuildpackName, "-m", DEFAULT_MEMORY_LIMIT, "-p", assets.NewAssets().Dora, "-d", config.AppsDomain).Wait(DEFAULT_TIMEOUT)).To(Exit(0))
			app_helpers.SetBackend(appName)

			Expect(cf.Cf("start", appName).Wait(CF_PUSH_TIMEOUT)).To(Exit(0))

			var envOutput string
			Eventually(func() string {
				envOutput = helpers.CurlApp(appName, "/env")
				return envOutput
			}, DEFAULT_TIMEOUT).Should(ContainSubstring(`"CF_INSTANCE_INDEX"=>"0"`))
			Expect(envOutput).To(MatchRegexp(`"CF_INSTANCE_IP"=>"[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+"`))
			Expect(envOutput).To(MatchRegexp(`"CF_INSTANCE_PORT"=>"[0-9]+"`))
			Expect(envOutput).To(MatchRegexp(`"CF_INSTANCE_ADDR"=>"[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+:[0-9]+"`))
			Expect(envOutput).To(MatchRegexp(`"CF_INSTANCE_PORTS"=>"\[(\{\\"external\\":[0-9]+,\\"internal\\":[0-9]+\},?)+\]"`))
		})

		It("generates an app usage 'started' event", func() {
			Expect(cf.Cf(
				"push",
				appName,
				"--no-start",
				"-b", config.RubyBuildpackName,
				"-m", DEFAULT_MEMORY_LIMIT,
				"-p", assets.NewAssets().Dora,
				"-d", config.AppsDomain).Wait(DEFAULT_TIMEOUT),
			).To(Exit(0))
			app_helpers.SetBackend(appName)

			Expect(cf.Cf("start", appName).Wait(CF_PUSH_TIMEOUT)).To(Exit(0))

			found, _ := lastAppUsageEvent(appName, "STARTED")
			Expect(found).To(BeTrue())
		})

		It("generates an app usage 'buildpack_set' event", func() {
			Expect(cf.Cf("push", appName, "--no-start", "-b", config.RubyBuildpackName, "-m", DEFAULT_MEMORY_LIMIT, "-p", assets.NewAssets().Dora, "-d", config.AppsDomain).Wait(DEFAULT_TIMEOUT)).To(Exit(0))
			app_helpers.SetBackend(appName)

			Expect(cf.Cf("start", appName).Wait(CF_PUSH_TIMEOUT)).To(Exit(0))

			found, matchingEvent := lastAppUsageEvent(appName, "BUILDPACK_SET")

			Expect(found).To(BeTrue())
			Expect(matchingEvent.Entity.BuildpackName).To(Equal("ruby_buildpack"))
			Expect(matchingEvent.Entity.BuildpackGuid).ToNot(BeZero())
		})
	})

	Describe("stopping", func() {
		BeforeEach(func() {
			Expect(cf.Cf("push", appName, "--no-start", "-b", config.RubyBuildpackName, "-m", DEFAULT_MEMORY_LIMIT, "-p", assets.NewAssets().Dora, "-d", config.AppsDomain).Wait(DEFAULT_TIMEOUT)).To(Exit(0))
			app_helpers.SetBackend(appName)

			Expect(cf.Cf("start", appName).Wait(CF_PUSH_TIMEOUT)).To(Exit(0))
		})

		It("makes the app unreachable", func() {
			Expect(cf.Cf("stop", appName).Wait(DEFAULT_TIMEOUT)).To(Exit(0))

			Eventually(func() string {
				return helpers.CurlAppRoot(appName)
			}, DEFAULT_TIMEOUT).Should(ContainSubstring("404"))
		})

		It("generates an app usage 'stopped' event", func() {
			Expect(cf.Cf("stop", appName).Wait(DEFAULT_TIMEOUT)).To(Exit(0))

			found, _ := lastAppUsageEvent(appName, "STOPPED")
			Expect(found).To(BeTrue())
		})

		Describe("and then starting", func() {
			It("makes the app reachable again", func() {
				Expect(cf.Cf("stop", appName).Wait(DEFAULT_TIMEOUT)).To(Exit(0))

				Expect(cf.Cf("start", appName).Wait(DEFAULT_TIMEOUT)).To(Exit(0))

				Eventually(func() string {
					return helpers.CurlAppRoot(appName)
				}, DEFAULT_TIMEOUT).Should(ContainSubstring("Hi, I'm Dora!"))
			})
		})
	})

	Describe("updating", func() {
		BeforeEach(func() {
			Expect(cf.Cf("push", appName, "--no-start", "-b", config.RubyBuildpackName, "-m", DEFAULT_MEMORY_LIMIT, "-p", assets.NewAssets().Dora, "-d", config.AppsDomain).Wait(DEFAULT_TIMEOUT)).To(Exit(0))
			app_helpers.SetBackend(appName)

			Expect(cf.Cf("start", appName).Wait(CF_PUSH_TIMEOUT)).To(Exit(0))
		})

		It("is reflected through another push", func() {
			Eventually(func() string {
				return helpers.CurlAppRoot(appName)
			}, DEFAULT_TIMEOUT).Should(ContainSubstring("Hi, I'm Dora!"))

			Expect(cf.Cf("push", appName, "--no-start", "-b", config.RubyBuildpackName, "-m", DEFAULT_MEMORY_LIMIT, "-p", assets.NewAssets().HelloWorld, "-d", config.AppsDomain).Wait(DEFAULT_TIMEOUT)).To(Exit(0))
			app_helpers.SetBackend(appName)
			Expect(cf.Cf("start", appName).Wait(CF_PUSH_TIMEOUT)).To(Exit(0))

			Eventually(func() string {
				return helpers.CurlAppRoot(appName)
			}, DEFAULT_TIMEOUT).Should(ContainSubstring("Hello, world!"))
		})
	})

	Describe("deleting", func() {
		BeforeEach(func() {
			Expect(cf.Cf("push", appName, "--no-start", "-b", config.RubyBuildpackName, "-m", DEFAULT_MEMORY_LIMIT, "-p", assets.NewAssets().Dora, "-d", config.AppsDomain).Wait(DEFAULT_TIMEOUT)).To(Exit(0))
			app_helpers.SetBackend(appName)

			Expect(cf.Cf("start", appName).Wait(CF_PUSH_TIMEOUT)).To(Exit(0))
		})

		It("removes the application", func() {
			Expect(cf.Cf("delete", appName, "-f", "-r").Wait(DEFAULT_TIMEOUT)).To(Exit(0))

			app := cf.Cf("app", appName).Wait(DEFAULT_TIMEOUT)
			Expect(app).To(Exit(1))
			Expect(app).To(Say("not found"))
		})

		It("makes the app unreachable", func() {
			Expect(cf.Cf("delete", appName, "-f", "-r").Wait(DEFAULT_TIMEOUT)).To(Exit(0))

			Eventually(func() string {
				return helpers.CurlAppRoot(appName)
			}, DEFAULT_TIMEOUT).Should(ContainSubstring("404"))
		})

		It("generates an app usage 'stopped' event", func() {
			Expect(cf.Cf("delete", appName, "-f", "-r").Wait(DEFAULT_TIMEOUT)).To(Exit(0))

			found, _ := lastAppUsageEvent(appName, "STOPPED")
			Expect(found).To(BeTrue())
		})
	})
})
