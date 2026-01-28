package streaming

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
)

type StreamingService struct { //nolint:revive // Name matches proto-generated type
	UnimplementedStreamingServiceServer
}

func NewStreamingService() *StreamingService {
	return &StreamingService{}
}

func (s *StreamingService) ListItems(req *ListItemsRequest, stream grpc.ServerStreamingServer[ItemResponse]) error {
	items := []Item{
		{Id: 1, Name: "Item 1", Category: req.Category},
		{Id: 2, Name: "Item 2", Category: req.Category},
		{Id: 3, Name: "Item 3", Category: req.Category},
		{Id: 4, Name: "Item 4", Category: req.Category},
		{Id: 5, Name: "Item 5", Category: req.Category},
	}

	limit := int(req.Limit)
	if limit == 0 || limit > len(items) {
		limit = len(items)
	}

	for i := 0; i < limit; i++ {
		if err := stream.Send(&ItemResponse{
			Item:    &items[i],
			Message: "Success",
		}); err != nil {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}

	return nil
}

func (s *StreamingService) WatchItems(req *WatchRequest, stream grpc.ServerStreamingServer[ItemEvent]) error {
	events := []ItemEvent{
		{
			EventType: "created",
			Item:      &Item{Id: req.StartId + 1, Name: fmt.Sprintf("Item %d", req.StartId+1), Category: "watched"},
			Timestamp: time.Now().Unix(),
		},
		{
			EventType: "updated",
			Item:      &Item{Id: req.StartId + 2, Name: fmt.Sprintf("Item %d", req.StartId+2), Category: "watched"},
			Timestamp: time.Now().Unix(),
		},
		{
			EventType: "deleted",
			Item:      &Item{Id: req.StartId + 3, Name: fmt.Sprintf("Item %d", req.StartId+3), Category: "watched"},
			Timestamp: time.Now().Unix(),
		},
	}

	for i := range events {
		if err := stream.Send(&events[i]); err != nil {
			return err
		}
		time.Sleep(200 * time.Millisecond)
	}

	return nil
}

func (s *StreamingService) Register(_ context.Context) error {
	return nil
}
