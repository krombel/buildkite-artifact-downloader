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
	buildkiteOrg      *string = flag.String("org", "matrix-dot-org", "BuildKite Organisation")
	buildkitePipeline *string = flag.String("pipeline", "riot-android", "BuildKite Pipeline")
	buildID           *int    = flag.Int("buildId", 0, "build ID which should be fetched")
	destPath          *string = flag.String("dest", destPathDefault, "Destination directory of artifact")
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
	log.Println("Download", buildID, ":", url)
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
	log.Println("Download", buildID, ",", jobID, ":", url)
	bodyBytes, err := getData(url)
	if err != nil {
		return nil, err
	}
	log.Println("Download succeeded")
	parsedResponse := []BuildkiteBuildArtifactInfo{}
	json.Unmarshal(bodyBytes, &parsedResponse)
	return parsedResponse, nil
}

func downloadArtifact(url string, destPath string) {
	// Create the file
	out, err := os.Create(destPath)
	if err != nil {
		log.Printf("Cannot create %s ('%s')\n", destPath, err)
		return
	}
	defer out.Close()

	// Get the data
	resp, err := netClient.Get(url)
	if err != nil {
		log.Printf("Cannot download to %s ('%s')\n", destPath, err)
		return
	}
	defer resp.Body.Close()

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		log.Printf("Cannot write to %s ('%s')\n", destPath, err)
		return
	}
}

func main() {
	flag.Parse()

	var err error
	if *buildID == 0 {
		*buildID, err = getLatestBuildID()
	}

	if *buildID == 0 {
		log.Fatal("BuildID unset and cannot be resolved")
		return
	}

	if *destPath == destPathDefault {
		*destPath = "./" + strconv.Itoa(*buildID) + "-"
	}

	buildInfo, err := getBuildInfo()
	if err != nil {
		fmt.Println(err)
		return
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
		log.Printf("Cannot find job with artifacts\n")
		return
	}

	var artifactInfo []BuildkiteBuildArtifactInfo
	artifactInfo, err = getArtifactInfo(foundJob.ID)
	if err != nil {
		log.Printf("%v\n", err)
		return
	}

	for _, artifact := range artifactInfo {
		log.Println("Start download of", artifact.Filename)
		downloadArtifact("https://buildkite.com"+artifact.URL, *destPath+artifact.Filename)
		log.Println(artifact.Filename, "downloaded")
	}
}
