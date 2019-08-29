package buildkiteArtifactDownloader

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	// DefaultDestinationPattern for artifact download
	DefaultDestinationPattern = "./<buildID>-<commitID>-<artifactFilename>"
)

// BuildkiteHandler object which handles all data to fetch artifacts from a pipeline
type BuildkiteHandler struct {
	buildkiteOrg      string
	buildkitePipeline string
	buildID           int
	artifactFilter    *regexp.Regexp
	destPattern       string
	netClient         *http.Client
}

// NewBuildkiteHandler constructs a new buildkite downloader instance
func NewBuildkiteHandler(
	buildkiteOrg string,
	buildkitePipeline string,
) *BuildkiteHandler {
	return &BuildkiteHandler{
		buildkiteOrg:      buildkiteOrg,
		buildkitePipeline: buildkitePipeline,

		netClient: &http.Client{
			Timeout: time.Second * 10,
		},
	}
}

// SetArtifactFilter sets (or deletes when nil passed) an artifact filter.
// Only matching files will be downloaded
func (bd *BuildkiteHandler) SetArtifactFilter(artifactFilter string) (err error) {
	var reArtifactFilter *regexp.Regexp
	if artifactFilter == "" {
		bd.artifactFilter = nil
		return
	}
	log.WithFields(log.Fields{
		"artifactFilter": artifactFilter,
	}).Debug("Compile artifact filter")

	reArtifactFilter, err = regexp.Compile(artifactFilter)
	if err != nil {
		return
	}
	bd.artifactFilter = reArtifactFilter
	return
}

// SetBuildID prefills buildID
func (bd *BuildkiteHandler) SetBuildID(buildID int) {
	bd.buildID = buildID
}

// SetDestinationPattern allows overwriting the default destination pattern
func (bd *BuildkiteHandler) SetDestinationPattern(destPattern string) {
	bd.destPattern = destPattern
	log.Info("Set DestPath: ", bd.destPattern)
}

func (bd *BuildkiteHandler) getDestinationPattern() string {
	if bd.destPattern != "" {
		return bd.destPattern
	}
	return DefaultDestinationPattern
}

func (bd *BuildkiteHandler) getDestinationPath(buildInfo BuildkiteBuildInfo, artifact BuildkiteBuildArtifactInfo) string {
	var output = bd.getDestinationPattern()

	log.WithFields(log.Fields{
		"destPattern":      output,
		"buildID":          bd.buildID,
		"commit":           buildInfo.CommitID[:8],
		"artifactFilename": artifact.Filename,
	}).Info("getDestinationPath")

	output = strings.ReplaceAll(
		output,
		`<buildID>`,
		strconv.Itoa(bd.buildID),
	)
	output = strings.ReplaceAll(
		output,
		`<commitID>`,
		buildInfo.CommitID[:8],
	)
	output = strings.ReplaceAll(
		output,
		`<artifactFilename>`,
		artifact.Filename,
	)

	log.WithFields(log.Fields{
		"output":  output,
		"buildID": bd.buildID,
	}).Info("ReplaceString end")

	return output
}

// resolveArtifacts returns an array of artifacts (filtered by artifactFilter)
func (bd *BuildkiteHandler) resolveArtifacts(job BuildkiteBuildJobInfo) ([]BuildkiteBuildArtifactInfo, error) {
	if job.ArtifactCount <= 0 {
		return nil, fmt.Errorf("Job contains no artifacts")
	}
	var err error

	var artifactInfo []BuildkiteBuildArtifactInfo
	artifactInfo, err = bd.getArtifactInfo(job.ID)
	if err != nil {
		return nil, err
	}

	var result []BuildkiteBuildArtifactInfo
	for _, artifact := range artifactInfo {
		if bd.artifactFilter != nil &&
			!bd.artifactFilter.MatchString(artifact.Filename) {
			log.WithFields(log.Fields{
				"buildID":          bd.buildID,
				"artifactFilename": artifact.Filename,
			}).Info("Skip artifact because it does not match artifact filter")
			continue
		}
		result = append(result, artifact)
	}

	return result, nil
}

// Start triggers a download of artifacts and returns
// the count of artifact downloads
func (bd *BuildkiteHandler) Start() (int, error) {
	var err error
	if bd.buildID == 0 {
		log.Debug("BuildId unset. Try resolving")
		bd.buildID, err = bd.getLatestBuildID()
		// ignore error as it is just meant to be a fallback
	}

	if bd.buildID == 0 {
		return 0, fmt.Errorf("BuildID unset and cannot be resolved")
	}

	buildInfo, err := bd.getBuildInfo()
	if err != nil {
		return 0, err
	}

	if buildInfo.State == "failed" {
		log.WithFields(log.Fields{
			"buildID": bd.buildID,
		}).Warn("Build failed. Abort")
		return 0, fmt.Errorf("Build %d failed", bd.buildID)
	}

	var artifacts []BuildkiteBuildArtifactInfo
	for _, job := range buildInfo.Jobs {
		if job.ArtifactCount <= 0 {
			// dont throw an error; just ignore jobs without artifacts
			log.WithFields(log.Fields{
				"buildID": bd.buildID,
				"jobID":   job.ID,
			}).Debug("Job contains no artifacts")
			continue
		}
		artifactsTmp, err := bd.resolveArtifacts(job)
		if err != nil {
			log.WithFields(log.Fields{
				"buildID": bd.buildID,
				"jobID":   job.ID,
			}).Info("resolving of artifacts failed")
		}
		if artifactsTmp == nil {
			log.WithFields(log.Fields{
				"buildID": bd.buildID,
				"jobID":   job.ID,
			}).Debug("No matching artifacts for job")
			continue
		}
		artifacts = append(artifacts, artifactsTmp...)
	}

	if len(artifacts) == 0 {
		log.WithFields(log.Fields{
			"buildID": bd.buildID,
		}).Warn("Cannot find matching artifacts")
		return 0, fmt.Errorf("Cannot find matching artifacts")
	}

	log.WithFields(log.Fields{
		"buildID":   bd.buildID,
		"artifacts": len(artifacts),
	}).Debug("Found artifacts")

	var downloadCount int
	for _, artifact := range artifacts {
		outPath := bd.getDestinationPath(*buildInfo, artifact)
		err := bd.downloadArtifact(artifact, outPath)
		if err != nil {
			log.Warn(err)
		} else {
			// there is no error so we assume, that the download succeeded
			downloadCount++
		}
	}
	return downloadCount, nil
}
