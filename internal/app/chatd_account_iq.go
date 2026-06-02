package app

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	waappv1 "github.com/byte-v-forge/wa-app/gen/go/byte/v/forge/waapp/v1"
)

const defaultAccountIQTimeout = 32 * time.Second

func (c *chatdClient) sendAccountIQ(ctx context.Context, state nativeState, input EngineAccountSettingsInput, appVersion string, request chatdNode) (chatdNode, error) {
	state.ChatStatic = ensureChatStatic(state.ChatStatic)
	privateKey, err := state.ChatStatic.privateBytes()
	if err != nil {
		return chatdNode{}, err
	}
	publicKey, err := state.ChatStatic.publicBytes()
	if err != nil {
		return chatdNode{}, err
	}
	identity, err := resolveLoginIdentity(input.RegisteredIdentityID, state)
	if err != nil {
		return chatdNode{}, err
	}
	conn, err := c.dial(ctx)
	if err != nil {
		return chatdNode{}, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(c.cfg.Timeout))
	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	routingInfo, err := decodeRoutingInfo(c.cfg.RoutingInfo)
	if err != nil {
		return chatdNode{}, err
	}
	loginPayload := defaultLoginPayload(identity, state, appVersion)
	keys, err := doNoiseHandshake(rw, privateKey, publicKey, loginPayload, routingInfo, c.cfg.MaxFrameBytes)
	if err != nil {
		return chatdNode{}, err
	}
	transport := chatdTransport{rw: rw, keys: keys, codec: c.codec, maxFrameBytes: c.cfg.MaxFrameBytes}
	if err := transport.sendNode(request); err != nil {
		return chatdNode{}, err
	}
	deadline := time.Now().Add(c.cfg.Timeout)
	requestID := request.Attrs["id"]
	for time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(time.Now().Add(time.Until(deadline)))
		node, err := transport.readNode()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				return chatdNode{}, fmt.Errorf("account settings iq timed out")
			}
			return chatdNode{}, err
		}
		if ack, ok := buildAckForNode(node); ok {
			_ = transport.sendNode(ack)
		}
		if node.Tag == "iq" && node.Attrs["id"] == requestID {
			return node, nil
		}
	}
	return chatdNode{}, fmt.Errorf("account settings iq timed out")
}

func chatdIQError(node chatdNode) error {
	if node.Attrs["type"] != "error" {
		return nil
	}
	message := "WA account settings request was rejected"
	if errorNode, ok := chatdChild(node, "error"); ok {
		if code := strings.TrimSpace(errorNode.Attrs["code"]); code != "" {
			message = message + " (code " + code + ")"
		}
	}
	return NewError(waappv1.WaErrorCode_WA_ERROR_CODE_REJECTED, message, false)
}

func chatdChild(node chatdNode, tag string) (chatdNode, bool) {
	for _, child := range chatdChildren(node) {
		if child.Tag == tag {
			return child, true
		}
	}
	return chatdNode{}, false
}

func chatdChildren(node chatdNode) []chatdNode {
	children, ok := node.Content.([]chatdNode)
	if !ok {
		return nil
	}
	return children
}

func chatdNodeValue(node chatdNode, name string) string {
	if value := strings.TrimSpace(node.Attrs[name]); value != "" {
		return value
	}
	if child, ok := chatdChild(node, name); ok {
		return chatdNodeText(child)
	}
	return ""
}

func chatdNodeText(node chatdNode) string {
	switch value := node.Content.(type) {
	case string:
		return strings.TrimSpace(value)
	case []byte:
		return strings.TrimSpace(string(value))
	default:
		return ""
	}
}

func chatdNodeBool(node chatdNode, name string) bool {
	switch strings.ToLower(chatdNodeValue(node, name)) {
	case "true", "1", "yes", "ok", "success":
		return true
	default:
		return false
	}
}

func chatdNodeDuration(node chatdNode, name string) time.Duration {
	value := strings.TrimSpace(chatdNodeValue(node, name))
	if value == "" {
		return 0
	}
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil || seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}
