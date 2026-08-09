package db

import "encoding/json"

var marshalCatalogItem = json.Marshal

type catalogCollector[T any] struct {
	items     []T
	bytes     int
	limit     int
	truncated bool
}

func (c *catalogCollector[T]) add(item T) (bool, error) {
	encoded, err := marshalCatalogItem(item)
	if err != nil {
		return false, err
	}
	if c.bytes+len(encoded)+1 > c.limit {
		c.truncated = true
		return false, nil
	}
	c.bytes += len(encoded) + 1
	c.items = append(c.items, item)
	return true, nil
}
