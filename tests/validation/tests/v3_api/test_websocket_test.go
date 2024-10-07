package v3_api

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

var namespace = struct {
	cluster   *Cluster
	shellURL  string
	pod       *Pod
	ns        string
}{}

func TestWebsocketLaunchKubectl(t *testing.T) {
	ws, err := createConnection(namespace.shellURL, []string{"base64.channel.k8s.io"})
	assert.NoError(t, err)
	defer ws.Close()

	logparse := NewWebsocketLogParse()
	go logparse.Receiver(ws, true)

	cmd := "kubectl version"
	checks := []string{"Client Version", "Server Version"}
	validateCommandExecution(t, ws, cmd, logparse, checks)
	logparse.LastMessage = ""

	cmd = "kubectl get ns -o name"
	checks = []string{"namespace/kube-system"}
	validateCommandExecution(t, ws, cmd, logparse, checks)
}

func TestWebsocketExecShell(t *testing.T) {
	urlBase := fmt.Sprintf("wss://%s/k8s/clusters/%s/api/v1/namespaces/%s/pods/%s/exec?container=%s",
		strings.TrimPrefix(CATTLE_TEST_URL, "https://"),
		namespace.cluster.ID,
		namespace.ns,
		namespace.pod.Name,
		namespace.pod.Containers[0].Name)

	paramsDict := url.Values{
		"stdout": {"1"},
		"stdin":  {"1"},
		"stderr": {"1"},
		"tty":    {"1"},
		"command": {"/bin/sh", "-c", "TERM=xterm-256color; export TERM; [ -x /bin/bash ] && ([ -x " +
			"/usr/bin/script ] && /usr/bin/script -q -c \"/bin/bash\" " +
			"/dev/null || exec /bin/bash) || exec /bin/sh "},
	}
	urlStr := urlBase + "&" + paramsDict.Encode()

	ws, err := createConnection(urlStr, []string{"base64.channel.k8s.io"})
	assert.NoError(t, err)
	defer ws.Close()

	logparse := NewWebsocketLogParse()
	go logparse.Receiver(ws, true)

	cmd := "ls"
	checks := []string{"bin", "boot", "dev"}
	validateCommandExecution(t, ws, cmd, logparse, checks)
}

func TestWebsocketViewLogs(t *testing.T) {
	urlBase := fmt.Sprintf("wss://%s/k8s/clusters/%s/api/v1/namespaces/%s/pods/%s/log?container=%s",
		strings.TrimPrefix(CATTLE_TEST_URL, "https://"),
		namespace.cluster.ID,
		namespace.ns,
		namespace.pod.Name,
		namespace.pod.Containers[0].Name)

	paramsDict := url.Values{
		"tailLines":  {"500"},
		"follow":     {"true"},
		"timestamps": {"true"},
		"previous":   {"false"},
	}
	urlStr := urlBase + "&" + paramsDict.Encode()

	ws, err := createConnection(urlStr, []string{"base64.binary.k8s.io"})
	assert.NoError(t, err)
	defer ws.Close()

	logparse := NewWebsocketLogParse()
	go logparse.Receiver(ws, false)

	time.Sleep(5 * time.Second) // Wait for some logs to be received

	fmt.Printf("\noutput:\n%s\n", logparse.LastMessage)
	assert.Contains(t, logparse.LastMessage, "websocket", "failed to view logs")
}

func sendACommand(ws *websocket.Conn, command string) {
	cmdEnc := base64.StdEncoding.EncodeToString([]byte(command))
	ws.WriteMessage(websocket.TextMessage, []byte("0"+cmdEnc))
	ws.WriteMessage(websocket.TextMessage, []byte("0DQ=="))
	time.Sleep(5 * time.Second)
}

func validateCommandExecution(t *testing.T, ws *websocket.Conn, command string, logObj *WebsocketLogParse, checking []string) {
	sendACommand(ws, command)
	fmt.Printf("\nshell command and output:\n%s\n", logObj.LastMessage)
	for _, check := range checking {
		assert.Contains(t, logObj.LastMessage, check, "failed to run the command")
	}
}

// Note: The following functions and types are assumed to be defined elsewhere in your Go codebase:
// - createConnection
// - NewWebsocketLogParse
// - WebsocketLogParse (type)
// - Cluster (type)
// - Pod (type)
// - CATTLE_TEST_URL (constant)

// TestMain would be used for setup and teardown, similar to the Python fixture
func TestMain(m *testing.M) {
	// Setup code here (create project, namespace, workload, etc.)
	// ...

	// Run tests
	m.Run()

	// Teardown code here
	// ...
}
