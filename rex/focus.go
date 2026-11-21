package rex

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type layout struct {
	SessionID string `json:"session_id"`
	Windows   []struct {
		ID     string `json:"window_id"`
		Layers []struct {
			Blocks []struct {
				Owner   string
				BlockID string `json:"block_id"`
			}
		}
	}
}

func (v layout) locate(block string) (window, owner string) {
	for _, win := range v.Windows {
		for _, layer := range win.Layers {
			for _, b := range layer.Blocks {
				if b.BlockID == block {
					return win.ID, b.Owner
				}
			}
		}
	}
	return "", ""
}

func (h Host) call(ctx context.Context, method string, args map[string]string, result any) error {
	if args == nil {
		args = map[string]string{}
	}
	payload, err := json.Marshal(args)
	if err != nil {
		return err
	}
	command := []string{"api", "call"}
	if session := args["session_id"]; session != "" {
		command = append(command, "-s", session)
	}
	output, err := h.command(ctx, append(command, method, string(payload))...).Output()
	if err != nil {
		return h.displayError(err)
	}
	if result != nil {
		return json.Unmarshal(output, result)
	}
	return nil
}

func (h Host) layout(ctx context.Context, id string) (layout, error) {
	var v layout
	err := h.call(ctx, "session.view", map[string]string{"session_id": id}, &v)
	if err == nil && v.SessionID != id {
		err = fmt.Errorf("Rex 返回了不同的会话")
	}
	return v, err
}

type appClient struct {
	ID        string `json:"client_id"`
	State     string `json:"connection_state"`
	Info      struct{ Kind, Label string }
	Principal struct{ Transport string }
}

func (h Host) appClients(ctx context.Context) ([]appClient, error) {
	var result struct{ Clients []appClient }
	if err := h.call(ctx, "client.list", nil, &result); err != nil {
		return nil, err
	}
	return result.Clients, nil
}

func clientOnHost(ctx context.Context, sourceID string, host Host) (string, error) {
	source, err := localHost.appClients(ctx)
	if err != nil {
		return "", err
	}
	var label string
	for _, c := range source {
		if c.ID == sourceID && c.Info.Kind == "app" {
			label = c.Info.Label
			break
		}
	}
	if label == "" {
		return "", fmt.Errorf("当前 Rex 应用需要客户端标签才能匹配远端设备")
	}
	for _, c := range source {
		if c.ID != sourceID && c.Info.Kind == "app" && c.Info.Label == label && c.State != "disconnected" {
			return "", fmt.Errorf("本机有多个同名 Rex 应用，请给当前设备使用独立标签")
		}
	}
	target, err := host.appClients(ctx)
	if err != nil {
		return "", err
	}
	var chosen string
	for _, c := range target {
		if c.Info.Kind != "app" || c.Info.Label != label || c.State == "disconnected" {
			continue
		}
		if chosen != "" {
			return "", fmt.Errorf("目标主机有多个同名 Rex 应用，请给当前设备使用独立标签")
		}
		chosen = c.ID
	}
	if chosen == "" {
		return "", fmt.Errorf("当前 Rex 应用未连接目标主机")
	}
	return chosen, nil
}

// Focus locates the current placement before acting. It focuses the exact
// block on its Rex server, then selects the tab on the initiating application.
func (c Context) Focus(parent context.Context, target Pane) error {
	if !c.Inside() || target.ID == "" || target.SessionID == "" || target.Host.ID == "" {
		return fmt.Errorf("缺少 Rex 窗格位置")
	}
	ctx, cancel := context.WithTimeout(parent, 12*time.Second)
	defer cancel()

	origin, err := localHost.layout(ctx, c.SessionID)
	if err != nil {
		return err
	}
	originWindow, owner := origin.locate(c.SidebarID)
	if originWindow == "" {
		return fmt.Errorf("Rex 找不到侧栏所在的窗格")
	}
	view := origin
	sameSession := target.Host.Server == "" && target.SessionID == c.SessionID
	if !sameSession {
		view, err = target.Host.layout(ctx, target.SessionID)
		if err != nil {
			return err
		}
	}
	window, _ := view.locate(target.ID)
	if window == "" || (sameSession && target.ID == c.SidebarID) {
		return fmt.Errorf("目标窗格已关闭或移出会话")
	}

	needsSelect := !sameSession || window != originWindow
	client := first(os.Getenv("REX_CLIENT"), owner)
	if needsSelect && client == "" {
		return fmt.Errorf("Rex 未提供当前应用的客户端标识")
	}
	if target.Host.Server != "" {
		client, err = clientOnHost(ctx, client, target.Host)
		if err != nil {
			return err
		}
	}
	if err := target.Host.call(ctx, "session.focus_block", map[string]string{
		"session_id": target.SessionID, "block_id": target.ID,
	}, nil); err != nil {
		return err
	}
	if !needsSelect {
		return nil
	}
	_, err = target.Host.command(ctx, "-C", client, "do", "session.select",
		"session_id="+target.SessionID, "window_id="+window).Output()
	if err != nil {
		return target.Host.displayError(err)
	}
	return nil
}
