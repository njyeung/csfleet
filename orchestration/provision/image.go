package provision

// ensureImage builds our CS2 server image, pulling the latest base
// (joedwards32/cs2) each time. This image is both what the server containers
// run and what we run SteamCMD inside, so provisioning builds it before
// anything else and keeps it current.
func ensureImage(p paths) error {
	logf("building %s (pulling latest base image)", cs2Image)
	return run("docker", "build", "--pull", "-t", cs2Image, p.root)
}
