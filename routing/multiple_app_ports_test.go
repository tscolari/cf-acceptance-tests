package routing

import (
	. "github.com/cloudfoundry/cf-acceptance-tests/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "github.com/cloudfoundry/cf-acceptance-tests/Godeps/_workspace/src/github.com/onsi/gomega"
	"github.com/cloudfoundry/cf-acceptance-tests/helpers/app_helpers"
	"github.com/cloudfoundry/cf-acceptance-tests/helpers/assets"
	. "github.com/cloudfoundry/cf-acceptance-tests/helpers/routing_helpers"
)

var _ = Describe(deaUnsupportedTag+"Multiple App Ports", func() {
	var (
		app             string
		latticeAppAsset = assets.NewAssets().LatticeApp
	)

	BeforeEach(func() {
		domain := config.AppsDomain

		app = GenerateAppName()
		cmd := fmt.Sprinf("%s --ports=7777,8888", app)
		PushAppNoStart(app, latticeAppAsset, config.GoBuildpackName, config.AppsDomain, CF_PUSH_TIMEOUT, cmd)
		UpdatePorts(app, []uint32{7777, 8888})
		EnableDiego(app)
		StartApp(app)
	})

	AfterEach(func() {
		app_helpers.AppReport(app, DEFAULT_TIMEOUT)
		DeleteApp(app, DEFAULT_TIMEOUT)
	})

	FContext("when app has multiple ports", func() {
		It("should listen on first port", func() {
			Eventually(func() string {
				return CurlApp(app, "/port")
			}, DEFAULT_TIMEOUT).Should(ContainSubstring("7777"))
		})
	})
})
