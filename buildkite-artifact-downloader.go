package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	destPathDefault string = "./<buildID>-<commitID>-<artifactFilename>"
)

var (
	netClient = &http.Client{
		Timeout: time.Second * 10,
	}
	artifactFilter      *string = flag.String("artifactFilter", "", "only download file which matches this regexp")
	artifactsDownloaded         = false
	buildkiteOrg        *string = flag.String("org", "matrix-dot-org", "BuildKite Organisation")
	buildkitePipeline   *string = flag.String("pipeline", "riot-android", "BuildKite Pipeline")
	buildID             *int    = flag.Int("buildId", 0, "build ID which should be fetched")
	destPath            *string = flag.String("dest", destPathDefault, "Destination directory of artifact")
	logLevel            *string = flag.String("log", "WARN", "One of DEBUG,INFO,WARN,ERROR")
)

type BuildkiteBuildJobInfo struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ArtifactCount int    `json:"artifact_count"`
	State         string `json:"state"`
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

func getData(url string) (bodyBytes []byte, err error) {
	buildResponse, err := netClient.Get(url)
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

func getLatestBuildID() (int, error) {
	resp, err := netClient.Head(
		"https://buildkite.com/" + *buildkiteOrg + "/" + *buildkitePipeline + "/builds/latest?branch=develop&state=passed",
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

func getBuildInfo() (*BuildkiteBuildInfo, error) {
	url := "https://buildkite.com/" + *buildkiteOrg + "/" + *buildkitePipeline + "/builds/" + strconv.Itoa(*buildID) + ".json?initial=true"
	log.WithFields(log.Fields{
		"buildID": *buildID,
		"url":     url,
	}).Debug("Start buildInfo download")
	bodyBytes, err := getData(url)
	if err != nil {
		return nil, err
	}
	log.WithFields(log.Fields{
		"buildID": *buildID,
		"url":     url,
	}).Debug("Download succeeded")
	parsedBuildResponse := BuildkiteBuildInfo{}
	json.Unmarshal(bodyBytes, &parsedBuildResponse)
	return &parsedBuildResponse, nil
}

func getArtifactInfo(jobID string) ([]BuildkiteBuildArtifactInfo, error) {
	url := "https://buildkite.com/organizations/" + *buildkiteOrg + "/pipelines/" + *buildkitePipeline + "/builds/" + strconv.Itoa(*buildID) + "/jobs/" + jobID + "/artifacts"
	log.WithFields(log.Fields{
		"buildID": *buildID,
		"jobID":   jobID,
		"url":     url,
	}).Info("Start artifactInfo download")
	bodyBytes, err := getData(url)
	if err != nil {
		return nil, err
	}
	log.WithFields(log.Fields{
		"buildID": *buildID,
		"jobID":   jobID,
		"url":     url,
	}).Info("Download succeeded")
	parsedResponse := []BuildkiteBuildArtifactInfo{}
	json.Unmarshal(bodyBytes, &parsedResponse)
	return parsedResponse, nil
}

func downloadArtifact(artifact BuildkiteBuildArtifactInfo, destPath string) error {
	if _, err := os.Stat(destPath); err == nil {
		return fmt.Errorf("Destination does already exist - do not download")
	}

	// Create the file
	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("Cannot create %s ('%s')", destPath, err)
	}
	defer out.Close()

	log.WithFields(log.Fields{
		"buildID":          *buildID,
		"artifactFilename": artifact.Filename,
		"destination":      destPath,
	}).Info("Start artifact download")

	// Get the data
	resp, err := netClient.Get("https://buildkite.com" + artifact.URL)
	if err != nil {
		return fmt.Errorf("Cannot download to %s ('%s')", destPath, err)
	}
	defer resp.Body.Close()

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("Cannot write to %s ('%s')", destPath, err)
	}

	log.WithFields(log.Fields{
		"buildID":          *buildID,
		"artifactFilename": artifact.Filename,
		"destination":      destPath,
	}).Info("Download finished")
	artifactsDownloaded = true
	return nil
}

func setLoglevel() {
	if *logLevel == "DEBUG" {
		log.SetLevel(log.DebugLevel)
	} else if *logLevel == "INFO" {
		log.SetLevel(log.InfoLevel)
	} else if *logLevel == "WARN" {
		log.SetLevel(log.WarnLevel)
	} else if *logLevel == "ERROR" {
		log.SetLevel(log.ErrorLevel)
	} else {
		log.WithFields(log.Fields{
			"loglevel": *logLevel,
		}).Fatal("Unsupported loglevel")
	}
}

func replaceStringByData(input string, buildInfo BuildkiteBuildInfo, artifact BuildkiteBuildArtifactInfo) string {
	var output string
	var re *regexp.Regexp
	re = regexp.MustCompile(`<buildID>`)
	output = re.ReplaceAllLiteralString(input, strconv.Itoa(*buildID))

	re = regexp.MustCompile(`<commitID>`)
	output = re.ReplaceAllLiteralString(output, buildInfo.CommitID[:8])

	re = regexp.MustCompile(`<artifactFilename>`)
	output = re.ReplaceAllLiteralString(output, artifact.Filename)

	return output
}

func buildkiteHandler() error {
	var err error
	if *buildID == 0 {
		log.Debug("BuildId unset. Try resolving")
		*buildID, err = getLatestBuildID()
		// ignore error as it is just meant to be a fallback
	}

	if *buildID == 0 {
		return fmt.Errorf("BuildID unset and cannot be resolved")
	}

	var reArtifactFilter *regexp.Regexp
	if *artifactFilter != "" {
		log.WithFields(log.Fields{
			"artifactFilter": *artifactFilter,
		}).Debug("Compile artifact filter")
		reArtifactFilter, err = regexp.Compile(*artifactFilter)
		if err != nil {
			log.WithFields(log.Fields{
				"artifactFilter": *artifactFilter,
			}).Fatal("Cannot parse artifactFilter")
		}
	}

	buildInfo, err := getBuildInfo()
	if err != nil {
		return err
	}

	if buildInfo.State == "failed" {
		log.WithFields(log.Fields{
			"buildID": *buildID,
		}).Warn("Build failed. Abort")
		return nil
	}

	var foundJob *BuildkiteBuildJobInfo
	for _, job := range buildInfo.Jobs {
		if job.ArtifactCount <= 0 {
			continue
		}
		foundJob = &job
		break
	}
	if foundJob == nil {
		log.WithFields(log.Fields{
			"buildID": *buildID,
		}).Warn("Cannot find job with artifacts")
		return fmt.Errorf("Cannot find job with artifacts")
	}

	var artifactInfo []BuildkiteBuildArtifactInfo
	artifactInfo, err = getArtifactInfo(foundJob.ID)
	if err != nil {
		return err
	}

	for _, artifact := range artifactInfo {
		if reArtifactFilter != nil && !reArtifactFilter.MatchString(artifact.Filename) {
			log.WithFields(log.Fields{
				"buildID":          *buildID,
				"artifactFilename": artifact.Filename,
			}).Info("Skip artifact because it does not match artifact filter")
			continue
		}
		outPath := replaceStringByData(*destPath, *buildInfo, artifact)
		err := downloadArtifact(artifact, outPath)
		if err != nil {
			log.Warn(err)
		}
	}
	return nil
}

func main() {
	flag.Parse()

	setLoglevel()

	err := buildkiteHandler()
	if err != nil {
		log.Warn(err)
	}

	// use exit code to respond if there are artifacts downloaded
	if artifactsDownloaded {
		os.Exit(0)
	} else {
		os.Exit(1)
	}
}
