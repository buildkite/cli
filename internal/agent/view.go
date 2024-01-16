package agent

import (
	"fmt"
	"strings"
)

func ParseMetadata(metadataList []string) (string, string) {
	var metadata, queue string

	for _, v := range metadataList {
		metadataStr := strings.Split(v, "=")
		if metadataStr[0] == "queue" {
			queue = metadataStr[1]
		} else {
			metadata += fmt.Sprintf("%s\n", v)
		}
	}

	return metadata, queue
}
