package buildkiteArtifactDownloader

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
)

var (
	netClient = &http.Client{
		Timeout: time.Second * 10,
	}
)

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
