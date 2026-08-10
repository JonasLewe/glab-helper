package gitlab

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

const maxItemsPerPage = 100

func parseArrayPages[T any](data []byte, itemName string, parseItem func([]byte) (T, error)) ([]T, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	items := make([]T, 0)

	for pageNumber := 1; ; pageNumber++ {
		var page []json.RawMessage
		err := decoder.Decode(&page)
		if err == io.EOF {
			if pageNumber == 1 {
				return nil, fmt.Errorf("response contained no JSON pages")
			}
			return items, nil
		}
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", pageNumber, err)
		}
		if page == nil {
			return nil, fmt.Errorf("page %d: expected an array", pageNumber)
		}

		for itemIndex, rawItem := range page {
			item, err := parseItem(rawItem)
			if err != nil {
				return nil, fmt.Errorf("page %d %s %d: %w", pageNumber, itemName, itemIndex+1, err)
			}
			items = append(items, item)
		}
	}
}
