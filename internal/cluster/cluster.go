package cluster

type Cluster struct {
	OrganizationSlug string
	ClusterID        string
	Queues           []Queue
	Name             string
	Description      string
}

type Queue struct {
	Id           string
	Name         string
	ActiveAgents int
}
