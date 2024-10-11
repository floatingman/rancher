//go:build (validation || infra.any || cluster.any) && !stress && !sanity && !extended

package websockets

import (
	"encoding/base64"
	"fmt"
	"github.com/rancher/rancher/tests/v2/actions/projects"
	"github.com/rancher/rancher/tests/v2/actions/workloads/deployment"
	pod "github.com/rancher/rancher/tests/v2/actions/workloads/pods"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/extensions/clusters"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	appsV1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	"net/url"
	"strings"
	"sync"
	"testing"

	ws "github.com/gorilla/websocket"
	"github.com/rancher/shepherd/clients/rancher"
	"github.com/rancher/shepherd/pkg/session"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

var (
	shellUrl      string
	podName       string
	containerName string
	testUrl       string
)

type WebsocketLogParse struct {
	lastMessage string
}

type WebsocketsTestSuite struct {
	suite.Suite
	client            *rancher.Client
	session           *session.Session
	cluster           *management.Cluster
	project           *management.Project
	namespace         *coreV1.Namespace
	createdDeployment *appsV1.Deployment
	adminToken        string
	host              string
}

func (w *WebsocketsTestSuite) SetupSuite() {
	testSession := session.NewSession()
	w.session = testSession

	client, err := rancher.NewClient("", testSession)
	require.NoError(w.T(), err)
	w.client = client

	log.Info("Getting cluster name from the config fileb")
	clusterName := client.RancherConfig.ClusterName
	require.NotEmptyf(w.T(), clusterName, "Cluster name to install should be set")
	clusterID, err := clusters.GetClusterIDByName(w.client, clusterName)
	require.NoError(w.T(), err, "Error getting cluster ID")
	w.cluster, err = w.client.Management.Cluster.ByID(clusterID)
	require.NoError(w.T(), err)

	project, namespace, err := projects.CreateProjectAndNamespace(w.client, w.cluster.ID)
	require.NoError(w.T(), err, "Error creating project and namespace")
	w.project = project
	w.namespace = namespace

	createdDeployment, err := deployment.CreateDeployment(w.client, w.cluster.ID, w.namespace.Name, 1, "", "", true, false)
	require.NoError(w.T(), err, "Error creating deployment")
	w.createdDeployment = createdDeployment

	w.adminToken = w.client.RancherConfig.AdminToken
	w.host = w.client.RancherConfig.Host
}

func (w *WebsocketsTestSuite) TearDownSuite() {
	w.session.Cleanup()
}

func (wp *WebsocketLogParse) readPump(wsc *ws.Conn) {
	for {
		_, message, err := wsc.ReadMessage()
		if err != nil {
			break
		}

		decodedMessage, _ := base64.StdEncoding.DecodeString(string(message[1:]))

		if strings.Contains(string(decodedMessage), "Client Version") || strings.Contains(string(decodedMessage), "Server Version") ||
			strings.Contains(string(decodedMessage), "bin") || strings.Contains(string(decodedMessage), "boot") || strings.Contains(string(decodedMessage),
			"dev") || strings.Contains(string(decodedMessage), "websocket") {
			wp.lastMessage = string(decodedMessage)
		}
	}
}

func createWebsocketConnection(shellUrl string, host string, token string) (*ws.Conn, error) {
	u := url.URL{Scheme: "wss", Host: host, Path: shellUrl}
	customDialer := NewCustomDialer(u.String(), token, true)
	c, resp, err := customDialer.Connect()

	if err != nil {
		fmt.Println("Failed to connect to the websocket:", err)
		fmt.Println(resp.Status)
		fmt.Println(resp.Body)
	}
	return c, err
}

func (w *WebsocketsTestSuite) TestWebsocketLaunchKubectl() {
	subSession := w.session.NewSession()
	defer subSession.Cleanup()

	w.Run("Verify kubectl is launched through a websocket", func() {

		wsc, err := createWebsocketConnection(shellUrl, w.host, w.adminToken)
		wlp := &WebsocketLogParse{}

		wg := sync.WaitGroup{}
		wg.Add(1)

		go func() {
			defer wg.Done()
			wlp.readPump(wsc)
		}()

		cmd := "kubectl version"
		sendACommand(wsc, cmd)
		wg.Wait()

		fmt.Println(wlp.lastMessage)
		wlp.lastMessage = ""

		cmd = "kubectl get ns -o name"
		sendACommand(wsc, cmd)
		wg.Wait()

		fmt.Println(wlp.lastMessage)
		wlp.lastMessage = ""
		assert.NoError(w.T(), err)
		err = wsc.Close()
		if err != nil {
			return
		}
	})
}

func (w *WebsocketsTestSuite) TestWebsocketExecShell() {
	subSession := w.session.NewSession()
	defer subSession.Cleanup()

	w.Run("Verify exec shell is launched through a websocket", func() {

		params := url.Values{}
		params.Add("stdout", "1")
		params.Add("stdin", "1")
		params.Add("stderr", "1")
		params.Add("tty", "1")
		params.Add("command", "/bin/sh -c TERM=xterm-256color; export TERM; [ -x /bin/bash ] && ([ -x '/usr/bin/script ] && /usr/bin/script -q -c '/bin/bash' '/dev/null || exec /bin/bash) || exec /bin/sh")

		podNames, err := pod.GetPodNamesFromDeployment(w.client, w.cluster.ID, w.namespace.Name, w.createdDeployment.Name)
		podName = podNames[0]

		containerName := w.createdDeployment.Spec.Template.Spec.Containers[0].Name

		u := url.URL{Path: "/k8s/clusters/" + w.cluster.ID + "/api/v1/namespaces/" + w.namespace.Name + "/pods/" + podName + "/exec?container=" + containerName, RawQuery: params.Encode()}

		wsc, err := createWebsocketConnection(u.String(), w.host, w.adminToken)
		wlp := &WebsocketLogParse{}

		wg := sync.WaitGroup{}
		wg.Add(1)

		go func() {
			defer wg.Done()
			wlp.readPump(wsc)
		}()

		cmd := "ls"
		sendACommand(wsc, cmd)
		wg.Wait()
		assert.NoError(w.T(), err)

		fmt.Println(wlp.lastMessage)
		wlp.lastMessage = ""

		err = wsc.Close()
		if err != nil {
			return
		}
	})
}

func (w *WebsocketsTestSuite) TestWebsocketViewLogs() {
	subSession := w.session.NewSession()
	defer subSession.Cleanup()

	w.Run("Verify logs are viewed through a websocket", func() {
		params := url.Values{}
		params.Add("tailLines", "500")
		params.Add("follow", "true")
		params.Add("timestamps", "true")
		params.Add("previous", "false")

		podNames, err := pod.GetPodNamesFromDeployment(w.client, w.cluster.ID, w.namespace.Name, w.createdDeployment.Name)
		podName = podNames[0]
		///k8s/clusters/c-m-hlx49zf4/api/v1/namespaces/auto-testns-yprnf/pods/auto-testdeployment-xjmie-86d5864f4c-h2k9m/log?follow=true&previous=false&tailLines=500&timestamps=true
		///k8s/clusters/c-m-hlx49zf4/api/v1/namespaces/auto-testns-lhvql/pods/auto-testdeployment-myyjg-6979d445f5-fng9c/log
		u := url.URL{Path: "/k8s/clusters/" + w.cluster.ID + "/api/v1/namespaces/" + w.namespace.Name + "/pods/" + podName + "/log", RawQuery: params.Encode()}
		wsc, err := createWebsocketConnection(u.String(), w.host, w.adminToken)
		wlp := &WebsocketLogParse{}

		wg := sync.WaitGroup{}
		wg.Add(1)

		go func() {
			defer wg.Done()
			wlp.readPump(wsc)
		}()

		fmt.Println(wlp.lastMessage)
		wlp.lastMessage = ""

		err = wsc.Close()
		if err != nil {
			return
		}
	})
}

func sendACommand(wsc *ws.Conn, command string) {
	cmdEnc := base64.StdEncoding.EncodeToString([]byte(command))
	err := wsc.WriteMessage(ws.BinaryMessage, []byte("0"+cmdEnc))
	if err != nil {
		return
	}
	err = wsc.WriteMessage(ws.TextMessage, []byte("0DQ=="))
	if err != nil {
		return
	}
}

func TestWebsocketsTestSuite(t *testing.T) {
	suite.Run(t, new(WebsocketsTestSuite))
}
