package buildkiteArtifactDownloader

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

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
}

// NewBuildkiteHandler constructs a new buildkite downloader instance
func NewBuildkiteHandler(
	buildkiteOrg string,
	buildkitePipeline string,
) *BuildkiteHandler {
	return &BuildkiteHandler{
		buildkiteOrg:      buildkiteOrg,
		buildkitePipeline: buildkitePipeline,
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
			"buildID": bd.buildID,
		}).Warn("Cannot find job with artifacts")
		return 0, fmt.Errorf("Cannot find job with artifacts")
	}

	var artifactInfo []BuildkiteBuildArtifactInfo
	artifactInfo, err = bd.getArtifactInfo(foundJob.ID)
	if err != nil {
		return 0, err
	}

	var downloadCount int
	for _, artifact := range artifactInfo {
		if bd.artifactFilter != nil &&
			!bd.artifactFilter.MatchString(artifact.Filename) {
			log.WithFields(log.Fields{
				"buildID":          bd.buildID,
				"artifactFilename": artifact.Filename,
			}).Info("Skip artifact because it does not match artifact filter")
			continue
		}
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
