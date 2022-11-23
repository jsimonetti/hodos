package routesync

import (
	"fmt"

	"github.com/jsimonetti/rtnetlink"
)

func ChangeMetric(conn *rtnetlink.Conn, msg rtnetlink.RouteMessage, metric uint32) error {
	msg.Flags = 0
	if err := conn.Route.Delete(&msg); err != nil {
		return fmt.Errorf("change metric error: unable to delete route: %w", err)
	}
	msg.Attributes.Priority = metric
	if err := conn.Route.Add(&msg); err != nil {
		return fmt.Errorf("change metric error: unable to add route: %w", err)
	}
	return nil
}
