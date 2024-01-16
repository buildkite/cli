package agent

import (
	"fmt"
	"strings"
)

func ParseMetadata(metadataList []string) (string, string) {
	var metadata, queue string

	// If no tags/only queue name (or default) is set - return a tilde (~) representing
	// no metadata key/value tags, along with the found queue name 
	if len(metadataList) == 1 {
		return "~", strings.Split(metadataList[0], "=")[1]
	} else {
		// We can't guarantee order of metadata key/value pairs, extract each pair
		// and the queue name when found in the respective element string
		for _, v := range metadataList {
			if strings.Contains(v, "queue=") {
				queue = strings.Split(v, "=")[1]
			} else {
				metadata += fmt.Sprintf("%s\n", v)
			}
		}
		return metadata, queue
	}	
}
