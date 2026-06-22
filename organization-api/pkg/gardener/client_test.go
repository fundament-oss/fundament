package gardener

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	if err := gardencorev1beta1.AddToScheme(scheme); err != nil {
		t.Fatalf("add gardener scheme: %v", err)
	}
	return scheme
}

// realClientWith builds a RealClient backed by a fake virtual-garden cluster
// holding the given objects.
func realClientWith(t *testing.T, objs ...client.Object) *RealClient {
	t.Helper()
	c := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(objs...).Build()
	return &RealClient{client: c, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func shoot(clusterID uuid.UUID) *gardencorev1beta1.Shoot {
	return &gardencorev1beta1.Shoot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-shoot",
			Namespace: "garden-proj",
			Labels:    map[string]string{labelClusterID: clusterID.String()},
		},
	}
}

func monitoringSecret(annotations map[string]string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "my-shoot" + monitoringSecretSuffix,
			Namespace:   "garden-proj",
			Annotations: annotations,
		},
		Data: map[string][]byte{
			"username": []byte("observer"),
			"password": []byte("s3cr3t"),
		},
	}
}

func TestRealClient_Logging(t *testing.T) {
	id := uuid.New()

	t.Run("returns vali url and credentials", func(t *testing.T) {
		c := realClientWith(t,
			shoot(id),
			monitoringSecret(map[string]string{valiURLAnnotation: "https://vali.example"}),
		)
		info, err := c.Logging(context.Background(), id)
		if err != nil {
			t.Fatalf("Logging: %v", err)
		}
		if info.URL != "https://vali.example" {
			t.Errorf("URL = %q, want https://vali.example", info.URL)
		}
		if info.Username != "observer" || info.Password != "s3cr3t" {
			t.Errorf("creds = (%q, %q), want (observer, s3cr3t)", info.Username, info.Password)
		}
	})

	t.Run("ErrNotFound when vali annotation absent", func(t *testing.T) {
		c := realClientWith(t,
			shoot(id),
			monitoringSecret(map[string]string{plutonoURLAnnotation: "https://plutono.example"}),
		)
		if _, err := c.Logging(context.Background(), id); !errors.Is(err, ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("ErrNotFound when shoot missing", func(t *testing.T) {
		c := realClientWith(t)
		if _, err := c.Logging(context.Background(), id); !errors.Is(err, ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})
}

func TestRealClient_Monitoring(t *testing.T) {
	id := uuid.New()
	c := realClientWith(t,
		shoot(id),
		monitoringSecret(map[string]string{plutonoURLAnnotation: "https://plutono.example"}),
	)
	info, err := c.Monitoring(context.Background(), id)
	if err != nil {
		t.Fatalf("Monitoring: %v", err)
	}
	if info.URL != "https://plutono.example" || info.Username != "observer" {
		t.Errorf("got %+v", info)
	}
}

func TestNoopClient_Logging(t *testing.T) {
	if _, err := (NoopClient{}).Logging(context.Background(), uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}
