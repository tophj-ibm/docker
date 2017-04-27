package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/docker/docker/integration-cli/checker"
	"github.com/docker/docker/integration-cli/registry"
	"github.com/go-check/check"
)

func init() {
	check.Suite(&DockerManifestSuite{
		ds: &DockerSuite{},
	})
}

type DockerManifestSuite struct {
	ds  *DockerSuite
	reg *registry.V2
}

func (s *DockerManifestSuite) SetUpSuite(c *check.C) {
	// make config.json if it doesn't exist, and add insecure registry to it
	os.Mkdir("/root/.docker/", 0770)
	if _, err := os.Stat("/root/.docker/config.json"); os.IsNotExist(err) {
		os.Create("/root/.docker/config.json")
	}
	f, err := os.OpenFile("/root/.docker/config.json", os.O_APPEND|os.O_WRONLY, 0600)
	c.Assert(err, checker.IsNil)
	defer f.Close()

	insecureRegistry := "{\"insecure-registries\" : [\"127.0.0.1:5000\"]}"
	_, err = f.WriteString(insecureRegistry)
	c.Assert(err, checker.IsNil)

	configLocation := "/root/.docker/config.json"
	_, err = os.Stat(configLocation)
	c.Assert(err, checker.IsNil)

}

func (s *DockerManifestSuite) TearDownSuite(c *check.C) {
	// intetionally empty
}

func (s *DockerManifestSuite) SetUpTest(c *check.C) {
	testRequires(c, DaemonIsLinux, registry.Hosting)

	// setup registry and populate it with two busybox images
	s.reg = setupRegistry(c, false, "", privateRegistryURL)

	image1 := fmt.Sprintf("%s/busybox", privateRegistryURL)
	image2 := fmt.Sprintf("%s/busybox2", privateRegistryURL)

	dockerCmd(c, "tag", "busybox", image1)
	dockerCmd(c, "tag", "busybox", image2)

	_, _, err := dockerCmdWithError("push", image1)
	c.Assert(err, checker.IsNil)

	_, _, err = dockerCmdWithError("push", image2)
	c.Assert(err, checker.IsNil)
}

func (s *DockerManifestSuite) TearDownTest(c *check.C) {
	if s.reg != nil {
		s.reg.Close()
	}
	s.ds.TearDownTest(c)
}

func (s *DockerManifestSuite) TestManifestCreate(c *check.C) {
	testRepo := "testrepo/busybox"

	out, _, _ := dockerCmdWithError("manifest", "create", testRepo, "busybox", "busybox:thisdoesntexist")
	c.Assert(out, checker.Contains, "manifest unknown")

	_, _, err := dockerCmdWithError("manifest", "create", testRepo, "busybox", "debian:jessie")
	c.Assert(err, checker.IsNil)

	splitRepo := strings.Split(testRepo, "/")
	c.Assert(len(splitRepo), checker.Equals, 2)

	manifestLocation := "/root/.docker/manifests/docker.io_" + splitRepo[0] + "_" + splitRepo[1] + "-latest"
	_, err = os.Stat(manifestLocation)
	c.Assert(err, checker.IsNil, check.Commentf("Manifest not found in ", manifestLocation))

}

func (s *DockerManifestSuite) TestManifestPush(c *check.C) {
	testRepo := "testrepo"
	testRepoRegistry := fmt.Sprintf("%s/%s", privateRegistryURL, testRepo)

	image1 := fmt.Sprintf("%s/busybox", privateRegistryURL)
	image2 := fmt.Sprintf("%s/busybox2", privateRegistryURL)

	dockerCmd(c, "manifest", "create", testRepoRegistry, image1, image2)

	dockerCmd(c, "manifest", "annotate", testRepoRegistry, image1, "--os", runtime.GOOS, "--arch", runtime.GOARCH)
	dockerCmd(c, "manifest", "annotate", testRepoRegistry, image2, "--os", runtime.GOOS, "--arch", runtime.GOARCH)

	out, _, err := dockerCmdWithError("manifest", "push", testRepoRegistry)
	c.Assert(err, checker.IsNil)
	successfulPush := "Succesfully pushed manifest list " + testRepo
	c.Assert(out, checker.Contains, successfulPush)
}

// tests a manifest inspect from a pushed image
func (s *DockerManifestSuite) TestManifestInspectPushedImage(c *check.C) {

	testRepo := "testrepo"
	testRepoRegistry := fmt.Sprintf("%s/%s", privateRegistryURL, testRepo)

	image1 := fmt.Sprintf("%s/busybox", privateRegistryURL)
	image2 := fmt.Sprintf("%s/busybox2", privateRegistryURL)

	dockerCmd(c, "manifest", "create", testRepoRegistry, image1, image2)
	dockerCmd(c, "manifest", "annotate", testRepoRegistry, image1, "--os", "linux", "--arch", "amd64")
	dockerCmd(c, "manifest", "annotate", testRepoRegistry, image2, "--os", "linux", "--arch", "amd64")

	dockerCmd(c, "manifest", "push", testRepoRegistry)

	out, _ := dockerCmd(c, "manifest", "inspect", testRepoRegistry)
	c.Assert(out, checker.Contains, "is a manifest list containing the following 2 manifest references:")
	c.Assert(out, checker.Contains, testRepo)

}

func (s *DockerManifestSuite) TestManifestAnnotate(c *check.C) {
	testRepo := "testrepo"
	testRepoRegistry := fmt.Sprintf("%s/%s", privateRegistryURL, testRepo)

	image1 := fmt.Sprintf("%s/busybox", privateRegistryURL)
	image2 := fmt.Sprintf("%s/busybox2", privateRegistryURL)

	dockerCmd(c, "manifest", "create", testRepoRegistry, image1, image2)

	// test with bad os / arch
	out, _, _ := dockerCmdWithError("manifest", "annotate", testRepoRegistry, image1, "--os", "bados", "--arch", "amd64")
	c.Assert(out, checker.Contains, "Manifest entry for image has unsupported os/arch combination")

	out, _, _ = dockerCmdWithError("manifest", "annotate", testRepoRegistry, image2, "--os", "linux", "--arch", "badarch")
	c.Assert(out, checker.Contains, "Manifest entry for image has unsupported os/arch combination")

	// now annotate correctly
	_, _, err := dockerCmdWithError("manifest", "annotate", testRepoRegistry, image1, "--os", "linux", "--arch", "amd64", "--cpuFeatures", "sse1", "--osFeatures", "osf1")
	c.Assert(err, checker.IsNil)
	_, _, err = dockerCmdWithError("manifest", "annotate", testRepoRegistry, image2, "--os", "freebsd", "--arch", "arm", "--cpuFeatures", "sse2", "--osFeatures", "osf2")
	c.Assert(err, checker.IsNil)

	dockerCmd(c, "manifest", "push", testRepoRegistry)

	out, _ = dockerCmd(c, "manifest", "inspect", testRepoRegistry)
	c.Assert(out, checker.Contains, "OS: linux")
	c.Assert(out, checker.Contains, "OS: freebsd")
	c.Assert(out, checker.Contains, "Arch: amd64")
	c.Assert(out, checker.Contains, "Arch: arm")
	c.Assert(out, checker.Contains, "CPU Features: sse")
	c.Assert(out, checker.Contains, "CPU Features: sse2")
	c.Assert(out, checker.Contains, "OS Features: osf1")
	c.Assert(out, checker.Contains, "OS Features: osf2")

}
