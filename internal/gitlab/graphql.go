package gitlab

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type graphQLError struct {
	Message string `json:"message"`
}

type graphQLResponse[T any] struct {
	Data   *T             `json:"data"`
	Errors []graphQLError `json:"errors"`
}

type workItemTypeJSON struct {
	ID   *string `json:"id"`
	Name *string `json:"name"`
}

type workItemJSON struct {
	ID           *string           `json:"id"`
	IID          json.RawMessage   `json:"iid"`
	WorkItemType *workItemTypeJSON `json:"workItemType"`
}

func decodeGraphQL[T any](data []byte) (*T, error) {
	var response graphQLResponse[T]
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	if len(response.Errors) > 0 {
		messages := make([]string, len(response.Errors))
		for index, graphQLError := range response.Errors {
			messages[index] = strings.TrimSpace(graphQLError.Message)
			if messages[index] == "" {
				messages[index] = "unknown GraphQL error"
			}
		}
		return nil, fmt.Errorf("GraphQL errors: %s", strings.Join(messages, "; "))
	}
	if response.Data == nil {
		return nil, fmt.Errorf("missing field %q", "data")
	}
	return response.Data, nil
}

func parseGraphQLIID(data []byte) (int64, error) {
	if len(data) == 0 || string(data) == "null" {
		return 0, fmt.Errorf("missing value")
	}
	text := string(data)
	if data[0] == '"' {
		if err := json.Unmarshal(data, &text); err != nil {
			return 0, err
		}
	}
	iid, err := strconv.ParseInt(text, 10, 64)
	if err != nil || iid < 1 {
		return 0, fmt.Errorf("must be a positive integer")
	}
	return iid, nil
}
