package cluster

// RegionStatusUpdate represents a region status update message
type RegionStatusUpdate struct {
	Provider      string `json:"provider"`
	AccountID     string `json:"account_id"`
	Region        string `json:"region"`
	Status        string `json:"status"`
	ResourceCount int    `json:"resource_count"`
}
