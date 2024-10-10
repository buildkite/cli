package api_types

type RESTResponse[T any] struct {
	Data []T `json:"data"`
}
