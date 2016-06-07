package v3

import (
	"encoding/json"
	"fmt"

	"github.com/cloudfoundry-incubator/cf-test-helpers/cf"
	"github.com/cloudfoundry-incubator/cf-test-helpers/generator"
	"github.com/cloudfoundry-incubator/cf-test-helpers/helpers"
	"github.com/cloudfoundry/cf-acceptance-tests/helpers/assets"
	. "github.com/cloudfoundry/cf-acceptance-tests/helpers/v3_helpers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gbytes"
	. "github.com/onsi/gomega/gexec"
)

var _ = Describe("route_mapping", func() {
	type ListResponse struct {
		TotalResults string `json:"guid"`
	}

	var (
		appName     string
		appGuid     string
		packageGuid string
		spaceGuid   string
		spaceName   string
		token       string
		response    ListResponse
		webProcess  Process
	)

	BeforeEach(func() {
		appName = generator.PrefixedRandomName("CATS-APP-")
		spaceName = context.RegularUserContext().Space
		spaceGuid = GetSpaceGuidFromName(spaceName)
		appGuid = CreateApp(appName, spaceGuid, `{"foo":"bar"}`)
		packageGuid = CreatePackage(appGuid)
		token := GetAuthToken()
		uploadUrl := fmt.Sprintf("%s%s/v3/packages/%s/upload", config.Protocol(), config.ApiEndpoint, packageGuid)

		UploadPackage(uploadUrl, assets.NewAssets().DoraZip, token)
		WaitForPackageToBeReady(packageGuid)

		dropletGuid := StageBuildpackPackage(packageGuid, "ruby_buildpack")
		WaitForDropletToStage(dropletGuid)
		AssignDropletToApp(appGuid, dropletGuid)

		processes := GetProcesses(appGuid, appName)
		webProcess = GetProcessByType(processes, "web")

		CreateAndMapRoute(appGuid, spaceName, helpers.LoadConfig().AppsDomain, webProcess.Name)

		StartApp(appGuid)

		Eventually(func() string {
			return helpers.CurlAppRoot(webProcess.Name)
		}, DEFAULT_TIMEOUT).Should(ContainSubstring("Hi, I'm Dora!"))

		Expect(cf.Cf("apps").Wait(DEFAULT_TIMEOUT)).To(Say(fmt.Sprintf("%s\\s+started", webProcess.Name)))
	})

	AfterEach(func() {
		FetchRecentLogs(appGuid, token, config)
		DeleteApp(appGuid)
	})

	FDescribe("Route mapping lifecycle", func() {
		Context("POST /v3/route_mappings", func() {
			It("creates a route mapping on an additional port", func() {
				addPortPath := fmt.Sprintf("/v3/processes/%s", webProcess.Guid)
				addPortBody := "{\"ports\": [8080, 1234], \"health_check\": {\"type\": \"process\"}}"

				Expect(cf.Cf("curl", addPortPath, "-X", "PATCH", "-d", addPortBody).Wait(DEFAULT_TIMEOUT)).To(Exit(0))

				fmt.Print("meow ADDED SECOND PORT")

				getRoutePath := fmt.Sprintf("/v2/routes?q=host:%s", helpers.LoadConfig().AppsDomain)
				routeBody := cf.Cf("curl", getRoutePath).Wait(DEFAULT_TIMEOUT).Out.Contents()
				routeJSON := struct {
					Resources []struct {
						Metadata struct {
							Guid string `json:"guid"`
						} `json:"metadata"`
					} `json:"resources"`
				}{}
				json.Unmarshal([]byte(routeBody), &routeJSON)
				routeGuid := routeJSON.Resources[0].Metadata.Guid
				addRouteBody := fmt.Sprintf(`
				{
					"relationships": {
						"app":   {"guid": "%s"},
						"route": {"guid": "%s"}
					},
					"app_port": 1234
				}`, appGuid, routeGuid)

				Expect(cf.Cf("curl", "/v3/route_mappings", "-X", "POST", "-d", addRouteBody).Wait(DEFAULT_TIMEOUT)).To(Exit(0))

				curlPath := fmt.Sprintf("/v3/route_mappings?app_guids=%s", appGuid)

				responseBuffer := cf.Cf("curl", curlPath)
				Expect(responseBuffer.Wait(DEFAULT_TIMEOUT)).To(Exit(0))

				err := json.Unmarshal(responseBuffer.Out.Contents(), &response)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.TotalResults).To(Equal(2))
			})
		})
	})
})
