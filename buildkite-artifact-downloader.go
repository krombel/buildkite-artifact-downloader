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

	"log"
)

const (
	destPathDefault string = "./<buildID>-"
)

var (
	netClient = &http.Client{
		Timeout: time.Second * 10,
	}
	buildkiteOrg        *string = flag.String("org", "matrix-dot-org", "BuildKite Organisation")
	buildkitePipeline   *string = flag.String("pipeline", "riot-android", "BuildKite Pipeline")
	buildID             *int    = flag.Int("buildId", 0, "build ID which should be fetched")
	destPath            *string = flag.String("dest", destPathDefault, "Destination directory of artifact")
	artifactFilter      *string = flag.String("artifactFilter", "", "only download file which matches this regexp")
	artifactsDownloaded         = false
)

type BuildkiteBuildJobInfo struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ArtifactCount int    `json:"artifact_count"`
	State         string `json:"state"`
}
type BuildkiteBuildInfo struct {
	State string `json:"state"`
	Jobs  []BuildkiteBuildJobInfo
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
	log.Println("Download", *buildID, ":", url)
	bodyBytes, err := getData(url)
	if err != nil {
		return nil, err
	}
	log.Println("Download succeeded")
	parsedBuildResponse := BuildkiteBuildInfo{}
	json.Unmarshal(bodyBytes, &parsedBuildResponse)
	return &parsedBuildResponse, nil
}

func getArtifactInfo(jobID string) ([]BuildkiteBuildArtifactInfo, error) {
	url := "https://buildkite.com/organizations/" + *buildkiteOrg + "/pipelines/" + *buildkitePipeline + "/builds/" + strconv.Itoa(*buildID) + "/jobs/" + jobID + "/artifacts"
	log.Println("Download", *buildID, ",", jobID, ":", url)
	bodyBytes, err := getData(url)
	if err != nil {
		return nil, err
	}
	log.Println("Download succeeded")
	parsedResponse := []BuildkiteBuildArtifactInfo{}
	json.Unmarshal(bodyBytes, &parsedResponse)
	return parsedResponse, nil
}

func downloadArtifact(url string, destPath string) error {
	if _, err := os.Stat(destPath); err == nil {
		return fmt.Errorf("Destination does already exist - do not download")
	}

	// Create the file
	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("Cannot create %s ('%s')", destPath, err)
	}
	defer out.Close()

	// Get the data
	resp, err := netClient.Get(url)
	if err != nil {
		return fmt.Errorf("Cannot download to %s ('%s')", destPath, err)
	}
	defer resp.Body.Close()

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("Cannot write to %s ('%s')", destPath, err)
	}

	artifactsDownloaded = true
	return nil
}

func buildkiteHandler() error {
	var err error
	if *buildID == 0 {
		log.Println("BuildId unset. Try resolving", *buildID)
		*buildID, err = getLatestBuildID()
		// ignore error as it is just meant to be a fallback
	}

	if *buildID == 0 {
		return fmt.Errorf("BuildID unset and cannot be resolved")
	}

	var reArtifactFilter *regexp.Regexp
	if *artifactFilter != "" {
		log.Println("Compile artifact filter")
		reArtifactFilter, err = regexp.Compile(*artifactFilter)
		if err != nil {
			return fmt.Errorf("Cannot parse artifactFilter")
		}
	}

	if *destPath == destPathDefault {
		*destPath = "./" + strconv.Itoa(*buildID) + "-"
	}

	buildInfo, err := getBuildInfo()
	if err != nil {
		return err
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
		return fmt.Errorf("Cannot find job with artifacts\n")
	}

	var artifactInfo []BuildkiteBuildArtifactInfo
	artifactInfo, err = getArtifactInfo(foundJob.ID)
	if err != nil {
		return err
	}

	for _, artifact := range artifactInfo {
		if reArtifactFilter != nil && !reArtifactFilter.MatchString(artifact.Filename) {
			log.Println("Skip", artifact.Filename, "because it does not match artifact filter")
			continue
		}
		log.Println("Start download of", artifact.Filename)
		err := downloadArtifact("https://buildkite.com"+artifact.URL, *destPath+artifact.Filename)
		if err != nil {
			log.Println("Error:", err)
		}
		log.Println(artifact.Filename, "downloaded")
	}
	return nil
}

func main() {
	flag.Parse()

	err := buildkiteHandler()
	if err != nil {
		log.Println("Error:", err)
	}

	// use exit code to respond if there are artifacts downloaded
	if artifactsDownloaded {
		os.Exit(0)
	} else {
		os.Exit(1)
	}
}
