package v3

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"

	"github.com/cloudfoundry-incubator/cf-test-helpers/cf"
	"github.com/cloudfoundry-incubator/cf-test-helpers/generator"
	"github.com/cloudfoundry-incubator/cf-test-helpers/runner"
	"github.com/cloudfoundry/cf-acceptance-tests/helpers/assets"
	. "github.com/cloudfoundry/cf-acceptance-tests/helpers/v3_helpers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gbytes"
	. "github.com/onsi/gomega/gexec"
)

var _ = Describe("package features", func() {
	var (
		appName            string
		appGuid            string
		packageGuid        string
		spaceGuid          string
		destinationAppGuid string
		token              string
		uploadUrl          string
	)

	BeforeEach(func() {
		appName = generator.PrefixedRandomName("CATS-APP-")
		spaceGuid = GetSpaceGuidFromName(context.RegularUserContext().Space)
		appGuid = CreateApp(appName, spaceGuid, "{}")
		packageGuid = CreatePackage(appGuid)
		token = GetAuthToken()
		uploadUrl = fmt.Sprintf("%s%s/v3/packages/%s/upload", config.Protocol(), config.ApiEndpoint, packageGuid)
	})

	AfterEach(func() {
		FetchRecentLogs(appGuid, token, config)
		DeleteApp(appGuid)
	})

	Context("with a valid package", func() {
		BeforeEach(func() {
			UploadPackage(uploadUrl, assets.NewAssets().DoraZip, token)
			WaitForPackageToBeReady(packageGuid)
		})

		AfterEach(func() {
			if destinationAppGuid != "" {
				DeleteApp(destinationAppGuid)
			}
		})

		It("can copy package bits to another app and download the package", func() {
			destinationAppName := generator.PrefixedRandomName("CATS-APP-")
			destinationAppGuid = CreateApp(destinationAppName, spaceGuid, "{}")

			// COPY
			copyUrl := fmt.Sprintf("/v3/apps/%s/packages?source_package_guid=%s", destinationAppGuid, packageGuid)
			session := cf.Cf("curl", copyUrl, "-X", "POST")
			bytes := session.Wait(DEFAULT_TIMEOUT).Out.Contents()
			var pac struct {
				Guid string `json:"guid"`
			}
			json.Unmarshal(bytes, &pac)
			copiedPackageGuid := pac.Guid

			WaitForPackageToBeReady(copiedPackageGuid)

			tmpdir, err := ioutil.TempDir(os.TempDir(), "package-download")
			Expect(err).ToNot(HaveOccurred())
			app_package_path := path.Join(tmpdir, destinationAppName)

			// DOWNLOAD
			session = cf.Cf("curl", fmt.Sprintf("/v3/packages/%s/download", copiedPackageGuid), "--output", app_package_path).Wait(DEFAULT_TIMEOUT)
			Expect(session).To(Exit(0))

			session = runner.Run("unzip", "-l", app_package_path)
			Expect(session.Wait(DEFAULT_TIMEOUT)).To(Exit(0))
			Expect(session.Out).To(Say("dora.rb"))
		})
	})

	Context("when the package contains files in unwriteable directories", func() {
		BeforeEach(func() {
			UploadPackage(uploadUrl, assets.NewAssets().JavaUnwriteableZip, token)
			WaitForPackageToBeReady(packageGuid)
		})

		It("can still stage the package", func() {
			dropletGuid := StageBuildpackPackage(packageGuid, "java_buildpack")
			dropletPath := fmt.Sprintf("/v3/droplets/%s", dropletGuid)

			Eventually(func() *Session {
				return cf.Cf("curl", dropletPath).Wait(DEFAULT_TIMEOUT)
			}, CF_PUSH_TIMEOUT).Should(Say("STAGED"))
		})
	})
})
