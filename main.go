package main

import (
	"flag"
	"os"

	downloader "./downloader"
	fdroidHandler "./fdroid-handler"
	log "github.com/sirupsen/logrus"
)

var (
	artifactFilter      *string = flag.String("artifactFilter", "", "only download file which matches this regexp")
	artifactsDownloaded         = false
	buildkiteOrg        *string = flag.String("org", "matrix-dot-org", "BuildKite Organisation")
	buildkitePipeline   *string = flag.String("pipeline", "riot-android", "BuildKite Pipeline")
	buildID             *int    = flag.Int("buildId", 0, "build ID which should be fetched")
	destPath            *string = flag.String("dest", downloader.DefaultDestinationPattern, "Destination directory of artifact")

	runFdroidUpdate  *bool   = flag.Bool("runFdroidUpdate", false, "if downloader should run \"fdroid update\" after download")
	fdroidVirtualEnv *string = flag.String("fdroidVENV", "", "optionaly declare the virtualenv the downloader should use")

	logLevel *string = flag.String("log", "WARN", "One of DEBUG,INFO,WARN,ERROR")
)

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

func main() {
	flag.Parse()

	//setLoglevel()

	buildkiteHandler := downloader.NewBuildkiteHandler(
		*buildkiteOrg, *buildkitePipeline,
	)
	if *destPath != "" {
		buildkiteHandler.SetDestinationPattern(*destPath)
	}

	if *buildID > 0 {
		buildkiteHandler.SetBuildID(*buildID)
	}
	if *artifactFilter != "" {
		err := buildkiteHandler.SetArtifactFilter(*artifactFilter)
		if err != nil {
			log.WithFields(log.Fields{
				"artifactFilter": *artifactFilter,
			}).Fatal("Cannot parse artifactFilter")
			os.Exit(2)
		}
	}

	downloads, err := buildkiteHandler.Start()
	if err != nil {
		log.Warn(err)
	}

	if downloads > 0 && *runFdroidUpdate {
		fh := fdroidHandler.NewFdroidHandler()
		if len(*fdroidVirtualEnv) > 0 {
			err = fh.SetFdroidVENV(*fdroidVirtualEnv)
			if err != nil {
				log.Error(err)
			}
		}
		fh.RunFdroidCommand("update")
		// TODO: Check if deploy is possible/configured
		fh.RunFdroidCommand("deploy")
	}

	// use exit code to respond if there are artifacts downloaded
	if downloads > 0 {
		os.Exit(0)
	} else {
		os.Exit(1)
	}
}
