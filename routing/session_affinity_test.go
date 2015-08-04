package routing

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cloudfoundry-incubator/cf-test-helpers/helpers"
	"github.com/cloudfoundry-incubator/cf-test-helpers/runner"
	"github.com/cloudfoundry/cf-acceptance-tests/helpers/assets"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const (
	VCAP_ID = "__VCAP_ID__"
)

var _ = Describe("Session Affinity", func() {
	var stickyAsset = assets.NewAssets().HelloRouting

	Context("when an app sets a JSESSIONID cookie", func() {
		var (
			appName string
		)
		BeforeEach(func() {
			appName = PushApp(stickyAsset)
		})

		AfterEach(func() {
			DeleteApp(appName)
		})

		Context("when an app has multiple instances", func() {
			BeforeEach(func() {
				ScaleAppInstances(appName, 3)
			})

			Context("when the client sends VCAP_ID and JSESSION cookies", func() {
				It("routes every request to the same instance", func() {
					var body string
					var header string

					Eventually(func() string {
						body, header = curlAppVerbose(appName, "/")
						return body
					}, DEFAULT_TIMEOUT).Should(ContainSubstring(fmt.Sprintf("Hello, %s", appName)))

					index := parseInstanceIndex(body)
					vCapCookie := parseVCAPCookie(header)

					headers := []string{
						fmt.Sprintf("Cookie:%s=%s", VCAP_ID, vCapCookie),
						fmt.Sprintf("Cookie:%s=%s", "JESSIONID", "some-jsession-id"),
					}

					Eventually(func() string {
						return curlAppWithHeaders(appName, "/", headers)
					}, DEFAULT_TIMEOUT).Should(ContainSubstring(fmt.Sprintf("Hello, %s at index: %d", appName, index)))
				})
			})
		})
	})

	Context("when an app does not set a JSESSIONID cookie", func() {
		var (
			helloWorldAsset = assets.NewAssets().HelloRouting

			appName string
		)
		BeforeEach(func() {
			appName = PushApp(helloWorldAsset)
		})

		AfterEach(func() {
			DeleteApp(appName)
		})

		Context("when an app has multiple instances", func() {
			BeforeEach(func() {
				ScaleAppInstances(appName, 3)
			})

			Context("when the client does not send VCAP_ID and JSESSION cookies", func() {
				It("routes requests round robin to all instances", func() {
					var body string
					var body2 string

					Eventually(func() string {
						body, _ = curlAppVerbose(appName, "/")
						return body
					}, DEFAULT_TIMEOUT).Should(ContainSubstring(fmt.Sprintf("Hello, %s", appName)))

					index := parseInstanceIndex(body)

					Eventually(func() string {
						body2 = helpers.CurlApp(appName, "/")
						return body
					}, DEFAULT_TIMEOUT).Should(ContainSubstring(fmt.Sprintf("Hello, %s", appName)))

					index2 := parseInstanceIndex(body2)
					Expect(index).ToNot(Equal(index2))
				})
			})
		})
	})

	Context("when two apps have different context paths", func() {
		var (
			app1Path = "/app1"
			app2Path = "/app2"
			app1     string
			app2     string
			domain   string
		)

		BeforeEach(func() {
			app1 = PushApp(stickyAsset)
			app2 = PushApp(stickyAsset)

			ScaleAppInstances(app1, 2)
			ScaleAppInstances(app2, 2)
			domain = "some-domain"

			MapRouteToApp(domain, app1Path, app1)
			MapRouteToApp(domain, app2Path, app2)
		})

		AfterEach(func() {
			DeleteApp(app1)
			DeleteApp(app2)
		})

		It("Sticky session should work", func() {
			var body string
			var header string

			Eventually(func() string {
				body, header = curlAppVerbose(domain, app1Path)
				return body
			}, DEFAULT_TIMEOUT).Should(ContainSubstring(fmt.Sprintf("Hello, %s", app1)))

			index := parseInstanceIndex(body)
			vCapCookie := parseVCAPCookie(header)

			headers := []string{
				fmt.Sprintf("Cookie:%s=%s", VCAP_ID, vCapCookie),
				fmt.Sprintf("Cookie:%s=%s", "JESSIONID", "some-jsession-id"),
			}

			fmt.Println("Print the header1:")
			fmt.Println(headers)

			Eventually(func() string {
				return curlAppWithHeaders(domain, app1Path, headers)
			}, DEFAULT_TIMEOUT).Should(ContainSubstring(fmt.Sprintf("Hello, %s at index: %d", app1, index)))

			// test the APP2
			Eventually(func() string {
				body, header = curlAppVerbose(domain, app2Path)
				return body
			}, DEFAULT_TIMEOUT).Should(ContainSubstring(fmt.Sprintf("Hello, %s", app2)))

			index = parseInstanceIndex(body)
			vCapCookie = parseVCAPCookie(header)

			headers = []string{
				fmt.Sprintf("Cookie:%s=%s", VCAP_ID, vCapCookie),
				fmt.Sprintf("Cookie:%s=%s", "JESSIONID", "some-jsession-id"),
			}

			fmt.Println("Print the header2:")
			fmt.Println(headers)

			Eventually(func() string {
				return curlAppWithHeaders(domain, app2Path, headers)
			}, DEFAULT_TIMEOUT).Should(ContainSubstring(fmt.Sprintf("Hello, %s at index: %d", app2, index)))
		})
	})

	FContext("when one app has a root path and another with a context path", func() {
		var (
			app2Path = "/app2"
			app1     string
			app2     string
			domain   string
		)

		BeforeEach(func() {
			app1 = PushApp(stickyAsset)
			app2 = PushApp(stickyAsset)

			ScaleAppInstances(app1, 2)
			ScaleAppInstances(app2, 2)
			domain = app1

			MapRouteToApp(domain, app2Path, app2)
		})

		AfterEach(func() {
			DeleteApp(app1)
			DeleteApp(app2)
		})

		It("Sticky session should work", func() {
			var body string
			var header string

			Eventually(func() string {
				body, header = curlAppVerbose(domain, app2Path)
				return body
			}, DEFAULT_TIMEOUT).Should(ContainSubstring(fmt.Sprintf("Hello, %s", app2)))

			index := parseInstanceIndex(body)
			vCapCookie := parseVCAPCookie(header)

			headers := []string{
				fmt.Sprintf("Cookie:%s=%s", VCAP_ID, vCapCookie),
				fmt.Sprintf("Cookie:%s=%s", "JESSIONID", "some-jsession-id"),
			}

			fmt.Println("Print the header1:")
			fmt.Println(header)

			Eventually(func() string {
				return curlAppWithHeaders(domain, app2Path, headers)
			}, DEFAULT_TIMEOUT).Should(ContainSubstring(fmt.Sprintf("Hello, %s at index: %d", app2, index)))

			Eventually(func() string {
				body, header = curlAppVerbose(domain, "/")
				return body
			}, DEFAULT_TIMEOUT).Should(ContainSubstring(fmt.Sprintf("Hello, %s", app1)))

			index = parseInstanceIndex(body)
			vCapCookie = parseVCAPCookie(header)

			headers = []string{
				fmt.Sprintf("Cookie:%s=%s", VCAP_ID, vCapCookie),
				fmt.Sprintf("Cookie:%s=%s", "JESSIONID", "some-jsession-id"),
			}

			fmt.Println("Print the header2:")
			fmt.Println(header)

			Eventually(func() string {
				return curlAppWithHeaders(domain, "/", headers)
			}, DEFAULT_TIMEOUT).Should(ContainSubstring(fmt.Sprintf("Hello, %s at index: %d", app1, index)))
		})
	})
})

func parseVCAPCookie(header string) string {
	strs := strings.SplitN(header, VCAP_ID+"=", -1)
	indexStr := strings.SplitN(strs[len(strs)-1], ";", -1)
	return indexStr[0]
}

func parseInstanceIndex(body string) int {
	strs := strings.SplitN(body, "index: ", -1)
	indexStr := strings.SplitN(strs[len(strs)-1], "!", -1)
	index, err := strconv.ParseInt(indexStr[0], 10, 0)
	Expect(err).ToNot(HaveOccurred())
	return int(index)
}

func curlAppVerbose(appName, path string) (string, string) {
	uri := helpers.AppUri(appName, path)
	curlCmd := runner.Curl(uri, "-v")
	runner.NewCmdRunner(curlCmd, helpers.CURL_TIMEOUT).Run()
	return string(curlCmd.Out.Contents()), string(curlCmd.Err.Contents())
}

func curlAppWithHeaders(appName, path string, headers []string) string {
	cmd := []string{}
	cmd = append(cmd, helpers.AppUri(appName, path))

	for _, header := range headers {
		cmd = append(cmd, "-H", header)
	}

	curlCmd := runner.Curl(cmd...)
	runner.NewCmdRunner(curlCmd, helpers.CURL_TIMEOUT).Run()
	Expect(string(curlCmd.Err.Contents())).To(HaveLen(0))
	return string(curlCmd.Out.Contents())
}
