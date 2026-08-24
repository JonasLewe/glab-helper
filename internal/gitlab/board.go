package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type Board struct {
	ID    int64
	Name  string
	Lists []BoardList
}

type BoardList struct {
	ID        int64
	LabelName string
}

type boardJSON struct {
	ID    *int64           `json:"id"`
	Name  *string          `json:"name"`
	Lists *[]boardListJSON `json:"lists"`
}

type boardListJSON struct {
	ID    *int64 `json:"id"`
	Label *struct {
		Name *string `json:"name"`
	} `json:"label"`
}

func (client *Client) ListBoards(ctx context.Context, projectID int64) ([]Board, error) {
	endpoint := fmt.Sprintf("projects/%d/boards?per_page=%d", projectID, maxItemsPerPage)
	output, err := client.output(ctx, "api", "--paginate", endpoint)
	if err != nil {
		return nil, fmt.Errorf("run glab board pagination: %w", err)
	}

	boards, err := parseArrayPages(output, "board", parseBoard)
	if err != nil {
		return nil, fmt.Errorf("decode paginated GitLab boards: %w", err)
	}
	return boards, nil
}

func parseBoard(data []byte) (Board, error) {
	var value boardJSON
	if err := json.Unmarshal(data, &value); err != nil {
		return Board{}, err
	}
	if value.ID == nil || *value.ID < 1 {
		return Board{}, fmt.Errorf("missing or invalid field %q", "id")
	}
	if value.Name == nil || strings.TrimSpace(*value.Name) == "" {
		return Board{}, fmt.Errorf("missing or invalid field %q", "name")
	}
	if value.Lists == nil {
		return Board{}, fmt.Errorf("field %q must be an array", "lists")
	}

	board := Board{ID: *value.ID, Name: *value.Name, Lists: make([]BoardList, 0, len(*value.Lists))}
	for index, rawList := range *value.Lists {
		list, err := parseBoardListJSON(rawList)
		if err != nil {
			return Board{}, fmt.Errorf("list %d: %w", index+1, err)
		}
		board.Lists = append(board.Lists, list)
	}
	return board, nil
}

func parseBoardList(data []byte) (BoardList, error) {
	var value boardListJSON
	if err := json.Unmarshal(data, &value); err != nil {
		return BoardList{}, err
	}
	return parseBoardListJSON(value)
}

func parseBoardListJSON(value boardListJSON) (BoardList, error) {
	if value.ID == nil || *value.ID < 1 {
		return BoardList{}, fmt.Errorf("missing or invalid field %q", "id")
	}
	list := BoardList{ID: *value.ID}
	if value.Label == nil {
		return list, nil
	}
	if value.Label.Name == nil || strings.TrimSpace(*value.Label.Name) == "" {
		return BoardList{}, fmt.Errorf("label has no valid name")
	}
	list.LabelName = *value.Label.Name
	return list, nil
}
