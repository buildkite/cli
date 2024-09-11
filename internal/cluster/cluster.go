package cluster

import (
	"github.com/buildkite/go-buildkite/v3/buildkite"
)

type Cluster struct {
	Color           *string                  `json:"color,omitempty"`
	CreatedAt       buildkite.Timestamp      `json:"created_at"`
	CreatedBy       buildkite.ClusterCreator `json:"created_by"`
	DefaultQueueID  *string                  `json:"default_queue_id"`
	DefaultQueueURL *string                  `json:"default_queue_url"`
	Description     *string                  `json:"description"`
	Emoji           *string                  `json:"emoji"`
	GraphQLID       string                   `json:"graphql_id"`
	ID              string                   `json:"id"`
	Name            string                   `json:"name"`
	Queues          []ClusterQueue
	QueuesURL       string `json:"queues_url"`
	URL             string `json:"url"`
	WebURL          string `json:"web_url"`
}

type ClusterQueue struct {
	ActiveAgents       int
	ClusterID          string                    `json:"cluster_id"`
	CreatedAt          buildkite.Timestamp       `json:"created_at"`
	CreatedBy          buildkite.ClusterCreator  `json:"created_by"`
	Description        *string                   `json:"description"`
	DispatchPaused     bool                      `json:"dispatch_paused"`
	DispatchPausedAt   *buildkite.Timestamp      `json:"dispatch_paused_at"`
	DispatchPausedBy   *buildkite.ClusterCreator `json:"dispatch_paused_by"`
	DispatchPausedNote *string                   `json:"dispatch_paused_note"`
	ID                 string                    `json:"id"`
	Key                string                    `json:"key"`
	URL                string                    `json:"url"`
	WebURL             string                    `json:"web_url"`
}
