package buildkiteArtifactDownloader

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/avast/apkverifier"
	log "github.com/sirupsen/logrus"
)

type BuildkiteBuildJobInfo struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}
type BuildkiteBuildInfo struct {
	State    string `json:"state"`
	CommitID string `json:"commit_id"`
	Jobs     []BuildkiteBuildJobInfo
}

type BuildkiteBuildArtifactInfo struct {
	State    string `json:"state"`
	Filename string `json:"file_name"`
	URL      string `json:"url"`
	SHA1sum  string `json:"sha1sum"`
}

func (bd *BuildkiteHandler) getLatestBuildID() (int, error) {
	resp, err := bd.netClient.Head(
		"https://buildkite.com/" + bd.buildkiteOrg + "/" + bd.buildkitePipeline + "/builds/latest?branch=develop&state=passed",
	)
	if err != nil {
		return 0, fmt.Errorf("Could not fetch buildID (%v)", err)
	}
	rp := regexp.MustCompile("[0-9]+$")
	match := rp.FindString(resp.Request.URL.String())
	if match == "" {
		return 0, fmt.Errorf("URL does not end with and buildID")
	}

	i, err := strconv.Atoi(match)
	if err != nil {
		return 0, fmt.Errorf("Could not parse buildID (%v)", err)
	}
	return i, nil
}

func (bd *BuildkiteHandler) getBuildInfo() (*BuildkiteBuildInfo, error) {
	url := "https://buildkite.com/" + bd.buildkiteOrg + "/" + bd.buildkitePipeline + "/builds/" + strconv.Itoa(bd.buildID) + ".json?initial=true"
	log.WithFields(log.Fields{
		"buildID": bd.buildID,
		"url":     url,
	}).Debug("Start buildInfo download")
	bodyBytes, err := bd.getData(url)
	if err != nil {
		return nil, err
	}
	log.WithFields(log.Fields{
		"buildID": bd.buildID,
		"url":     url,
	}).Debug("Download succeeded")
	parsedBuildResponse := BuildkiteBuildInfo{}
	json.Unmarshal(bodyBytes, &parsedBuildResponse)
	return &parsedBuildResponse, nil
}

func (bd *BuildkiteHandler) getArtifactInfo(jobID string) ([]BuildkiteBuildArtifactInfo, error) {
	url := "https://buildkite.com/organizations/" + bd.buildkiteOrg + "/pipelines/" + bd.buildkitePipeline + "/builds/" + strconv.Itoa(bd.buildID) + "/jobs/" + jobID + "/artifacts"
	log.WithFields(log.Fields{
		"buildID": bd.buildID,
		"jobID":   jobID,
		"url":     url,
	}).Info("Start artifactInfo download")
	bodyBytes, err := bd.getData(url)
	if err != nil {
		return nil, err
	}
	log.WithFields(log.Fields{
		"buildID": bd.buildID,
		"jobID":   jobID,
		"url":     url,
	}).Info("Download succeeded")
	parsedResponse := []BuildkiteBuildArtifactInfo{}
	json.Unmarshal(bodyBytes, &parsedResponse)
	return parsedResponse, nil
}

func (bd *BuildkiteHandler) getData(url string) (bodyBytes []byte, err error) {
	buildResponse, err := bd.netClient.Get(url)
	if err != nil {
		log.Fatal("GET failed", err)
		return nil, err
	}
	defer buildResponse.Body.Close()

	if buildResponse.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Could not get data")
	}

	bodyBytes, err = ioutil.ReadAll(buildResponse.Body)
	if err != nil {
		return nil, err
	}
	return bodyBytes, nil
}

func (bd *BuildkiteHandler) downloadArtifact(artifact BuildkiteBuildArtifactInfo, destPath string) error {
	if _, err := os.Stat(destPath); err == nil {
		return fmt.Errorf("Destination does already exist - do not download")
	}

	tmpFile, err := ioutil.TempFile(os.TempDir(), "buildkite-artifact-")
	if err != nil {
		log.WithFields(log.Fields{
			"buildID":          bd.buildID,
			"artifactFilename": artifact.Filename,
			"destination":      destPath,
			"error":            err,
		}).Fatal("Cannot create temporary file")
	}
	// Remember to clean up the file afterwards
	defer os.Remove(tmpFile.Name())

	log.WithFields(log.Fields{
		"buildID":          bd.buildID,
		"artifactFilename": artifact.Filename,
		"destination":      destPath,
	}).Info("Start artifact download")

	// Get the data
	resp, err := bd.netClient.Get("https://buildkite.com" + artifact.URL)
	if err != nil {
		return fmt.Errorf("Cannot download to %s ('%s')", destPath, err)
	}
	defer resp.Body.Close()

	// Write the body to file
	_, err = io.Copy(tmpFile, resp.Body)
	if err != nil {
		if e, ok := err.(net.Error); ok && e.Timeout() {
			log.WithFields(log.Fields{
				"buildID":          bd.buildID,
				"artifactFilename": artifact.Filename,
				"destination":      destPath,
				"error":            e,
			}).Warn("Download interrupted. Timeout occured")
			// This was a timeout
		} else {
			log.WithFields(log.Fields{
				"buildID":          bd.buildID,
				"artifactFilename": artifact.Filename,
				"destination":      destPath,
				"error":            err,
			}).Warn("Download interrupted. Download not stored")
			return fmt.Errorf("Cannot write to temp file %s ('%s')", tmpFile.Name(), err)
		}
	}

	// Close the file
	if err := tmpFile.Close(); err != nil {
		log.WithFields(log.Fields{
			"buildID":          bd.buildID,
			"artifactFilename": artifact.Filename,
			"tmpFile":          tmpFile.Name(),
			"error":            err,
		}).Fatal("Cannot close tmpfile")
	}

	if strings.HasSuffix(destPath, ".apk") {
		log.WithFields(log.Fields{
			"buildID":          bd.buildID,
			"artifactFilename": artifact.Filename,
			"tmpFile":          tmpFile.Name(),
		}).Info("Validate APK")
		_, err := apkverifier.Verify(tmpFile.Name(), nil)
		if err != nil {
			log.WithFields(log.Fields{
				"buildID":          bd.buildID,
				"artifactFilename": artifact.Filename,
				"tmpFile":          tmpFile.Name(),
				"error":            err,
			}).Warn("Verification of APK failed: %s", err.Error())
			return fmt.Errorf("Verification of APK failed: %s", err.Error())
		}
	}

	data, err := ioutil.ReadFile(tmpFile.Name())
	if err != nil {
		log.WithFields(log.Fields{
			"buildID":          bd.buildID,
			"artifactFilename": artifact.Filename,
			"tmpFile":          tmpFile.Name(),
			"error":            err,
		}).Warn("Cannot read tmpfile")
		return fmt.Errorf("Cannot read tmpfile %s ('%s')", tmpFile.Name(), err)
	}
	err = ioutil.WriteFile(destPath, data, 0644)
	if err != nil {
		log.WithFields(log.Fields{
			"buildID":          bd.buildID,
			"artifactFilename": artifact.Filename,
			"destination":      destPath,
			"error":            err,
		}).Warn("Cannot write to destination")
		return fmt.Errorf("Cannot write to %s ('%s')", destPath, err)
	}

	log.WithFields(log.Fields{
		"buildID":          bd.buildID,
		"artifactFilename": artifact.Filename,
		"destination":      destPath,
	}).Info("Download finished")
	return nil
}
