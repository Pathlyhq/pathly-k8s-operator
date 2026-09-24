package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	pathlyv1 "github.com/pathlyhq/pathly-k8s-operator/api/v1alpha1"
	pathlyclient "github.com/pathlyhq/pathly-k8s-operator/internal/client"
)

const (
	finalizerName = "pathly.pathlyhq.com/finalizer"
	errNoToken    = "credentials Secret has no token (expected keys: token or apiToken)"
)

// ClientFromConnection builds a Pathly API client from a PathlyConnection in the same namespace.
func ClientFromConnection(ctx context.Context, kube client.Client, namespace string, ref corev1.LocalObjectReference) (*pathlyclient.Client, error) {
	if ref.Name == "" {
		return nil, fmt.Errorf("connectionRef.name is required")
	}
	conn := &pathlyv1.PathlyConnection{}
	if err := kube.Get(ctx, types.NamespacedName{Namespace: namespace, Name: ref.Name}, conn); err != nil {
		return nil, fmt.Errorf("get PathlyConnection %s/%s: %w", namespace, ref.Name, err)
	}
	token, err := tokenFromSecret(ctx, kube, namespace, conn.Spec.SecretRef)
	if err != nil {
		return nil, err
	}
	baseURL := pathlyclient.DefaultBaseURL
	if conn.Spec.BaseURL != nil && strings.TrimSpace(*conn.Spec.BaseURL) != "" {
		baseURL = *conn.Spec.BaseURL
	}
	return pathlyclient.New(baseURL, token), nil
}

func tokenFromSecret(ctx context.Context, kube client.Client, namespace string, ref corev1.SecretKeySelector) (string, error) {
	secret := &corev1.Secret{}
	nn := types.NamespacedName{Namespace: namespace, Name: ref.Name}
	if err := kube.Get(ctx, nn, secret); err != nil {
		return "", fmt.Errorf("get credentials Secret %s/%s: %w", namespace, ref.Name, err)
	}
	key := ref.Key
	if key == "" {
		key = "token"
	}
	raw, ok := secret.Data[key]
	if !ok || len(raw) == 0 {
		if key != "apiToken" {
			raw, ok = secret.Data["apiToken"]
		}
		if !ok || len(raw) == 0 {
			return "", fmt.Errorf("%s", errNoToken)
		}
	}
	return strings.TrimSpace(string(raw)), nil
}

func idempotencyKey(uid interface{}, action string) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%v:%s", uid, action)))
	return hex.EncodeToString(h[:16])
}

func CopyStringPtr(in *string) *string {
	if in == nil {
		return nil
	}
	v := *in
	return &v
}

func CopyInt64Ptr(in *int64) *int64 {
	if in == nil {
		return nil
	}
	v := *in
	return &v
}

func CopyBoolPtr(in *bool) *bool {
	if in == nil {
		return nil
	}
	v := *in
	return &v
}

func CopyFloat64Ptr(in *float64) *float64 {
	if in == nil {
		return nil
	}
	v := *in
	return &v
}

func stringPtr(s string) *string { return &s }
