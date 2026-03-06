package kube

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// KubeClient abstracts access to a Kubernetes API server.
type KubeClient interface {
	Do(ctx context.Context, method, path string, body io.Reader) (statusCode int, responseBody io.Reader, err error)
}

// MockKubeClient returns hardcoded Kubernetes API responses for development and testing.
type MockKubeClient struct{}

func (m *MockKubeClient) Do(_ context.Context, _, path string, _ io.Reader) (int, io.Reader, error) {
	if strings.Contains(path, "customresourcedefinitions") {
		return 200, strings.NewReader(mockCRDList), nil
	}
	return 200, strings.NewReader(mockEmptyList), nil
}

// RealKubeClient connects to a real Kubernetes API server using a kubeconfig.
// Not yet implemented — placeholder for future use.
type RealKubeClient struct{}

func (r *RealKubeClient) Do(_ context.Context, _, _ string, _ io.Reader) (int, io.Reader, error) {
	return 0, nil, fmt.Errorf("real kubernetes client not yet implemented")
}

const mockCRDList = `{
  "apiVersion": "apiextensions.k8s.io/v1",
  "kind": "CustomResourceDefinitionList",
  "metadata": {
    "resourceVersion": "1"
  },
  "items": [
    {
      "apiVersion": "apiextensions.k8s.io/v1",
      "kind": "CustomResourceDefinition",
      "metadata": {
        "name": "widgets.example.com"
      },
      "spec": {
        "group": "example.com",
        "names": {
          "kind": "Widget",
          "listKind": "WidgetList",
          "plural": "widgets",
          "singular": "widget"
        },
        "scope": "Namespaced",
        "versions": [
          {
            "name": "v1",
            "schema": {
              "openAPIV3Schema": {
                "properties": {
                  "spec": {
                    "properties": {
                      "color": {"type": "string"},
                      "size": {"type": "integer"}
                    },
                    "type": "object"
                  }
                },
                "type": "object"
              }
            },
            "served": true,
            "storage": true
          }
        ]
      }
    }
  ]
}`

const mockEmptyList = `{
  "apiVersion": "v1",
  "kind": "List",
  "metadata": {
    "resourceVersion": ""
  },
  "items": []
}`
