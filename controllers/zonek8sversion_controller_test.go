package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	controlv1 "github.com/appuio/control-api/apis/v1"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	//"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/version"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"

	restfake "k8s.io/client-go/rest/fake"
)

func Test_ZoneK8sVersionReconciler_Reconcile(t *testing.T) {
	zoneID := "c-appuio-test-cluster"
	zone := controlv1.Zone{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
			Labels: map[string]string{
				upstreamZoneIdentifierLabelKey: zoneID,
			},
		},
		Data: controlv1.ZoneData{
			Features: map[string]string{
				"foo": "bar",
			},
		},
	}
	otherZone := controlv1.Zone{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-2",
			Labels: map[string]string{
				upstreamZoneIdentifierLabelKey: "c-appuio-other-cluster",
			},
		},
		Data: controlv1.ZoneData{
			Features: map[string]string{
				"foo": "bar",
			},
		},
	}
	foreignClient, _, _ := prepareClient(t, &zone, &otherZone)
	cv := makeClusterVersion("4.16.19", []configv1.UpdateHistory{})
	client, scheme, recorder := prepareClient(t, &cv)

	version := version.Info{
		Major: "1",
		Minor: "29",
	}
	marshaledVersion, err := json.Marshal(version)
	require.NoError(t, err)

	// Setup a fake REST client which returns the marshaled version.Info
	// on requests on /version
	restclient := restfake.RESTClient{
		NegotiatedSerializer: serializer.WithoutConversionCodecFactory{CodecFactory: clientgoscheme.Codecs},
		Client: restfake.CreateHTTPClient(
			func(req *http.Request) (*http.Response, error) {
				if req.Method == "GET" && req.URL.Path == "/version" {
					resp := http.Response{
						Header:        make(http.Header, 0),
						Body:          io.NopCloser(bytes.NewBuffer(marshaledVersion)),
						ContentLength: int64(len(marshaledVersion)),
						Status:        "200 OK",
						StatusCode:    200,
						Proto:         "HTTP/1.1",
						ProtoMajor:    1,
						ProtoMinor:    1,
						Request:       req,
					}
					resp.Header.Add("Content-Type", "application/json; charset=utf-8")
					return &resp, nil
				}
				return nil, fmt.Errorf("Unexpected request")
			},
		),
	}

	subject := ZoneK8sVersionReconciler{
		Client:        client,
		Scheme:        scheme,
		Recorder:      recorder,
		ForeignClient: foreignClient,
		RESTClient:    &restclient,
		ZoneID:        zoneID,
	}

	// NOTE(sg): ClusterVersion is a singleton which is always named
	// version
	_, err = subject.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "version"}})
	require.NoError(t, err)

	// Get updated zone from the foreign client to check added fields
	updatedZone := controlv1.Zone{}
	err = foreignClient.Get(context.Background(), types.NamespacedName{Name: "test"}, &updatedZone)
	require.NoError(t, err)
	require.Equal(t, "4.16", updatedZone.Data.Features[openshiftVersionFeatureKey], "OCP version is set")
	require.Equal(t, "1.29", updatedZone.Data.Features[kubernetesVersionFeatureKey], "K8s version is set")
	require.Equal(t, "bar", updatedZone.Data.Features["foo"], "Unrelated fields are left in place")

	// Verify that unrelated zone isn't updated
	updatedOtherZone := controlv1.Zone{}
	err = foreignClient.Get(context.Background(), types.NamespacedName{Name: "test-2"}, &updatedOtherZone)
	require.NoError(t, err)
	require.Equal(t, controlv1.Features{"foo": "bar"}, updatedOtherZone.Data.Features, "unrelated zones are untouched")
}

func Test_extractOpenShiftVersion(t *testing.T) {
	cv := makeClusterVersion("4.16.5", []configv1.UpdateHistory{})
	v, err := extractOpenShiftVersion(&cv)
	require.NoError(t, err)
	require.Equal(t, "4", v.Major)
	require.Equal(t, "16", v.Minor)
}

func Test_extractOpenShiftVersionWithHistory(t *testing.T) {
	history := []configv1.UpdateHistory{
		configv1.UpdateHistory{
			State:          "Partial",
			StartedTime:    metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
			CompletionTime: nil,
			Version:        "4.16.5",
		},
		configv1.UpdateHistory{
			State:          "Completed",
			StartedTime:    metav1.Time{Time: time.Now().Add(-3 * time.Hour)},
			CompletionTime: &metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
			Version:        "4.15.25",
			Verified:       true,
		},
		configv1.UpdateHistory{
			State:          "Completed",
			StartedTime:    metav1.Time{Time: time.Now().Add(-24 * time.Hour)},
			CompletionTime: &metav1.Time{Time: time.Now().Add(-23 * time.Hour)},
			Version:        "4.14.29",
			Verified:       true,
		},
	}
	cv := makeClusterVersion("4.16.5", history)
	v, err := extractOpenShiftVersion(&cv)
	require.NoError(t, err)
	require.Equal(t, "4", v.Major)
	require.Equal(t, "15", v.Minor, "Prefer completed upgrade in history over desired upgrade")
}

func makeClusterVersion(desired string, history []configv1.UpdateHistory) configv1.ClusterVersion {
	return configv1.ClusterVersion{
		ObjectMeta: metav1.ObjectMeta{
			Name: "version",
		},
		Spec: configv1.ClusterVersionSpec{
			ClusterID: "ocpID",
			Channel:   "stable-4.16",
		},
		Status: configv1.ClusterVersionStatus{
			Desired: configv1.Release{
				Version: desired,
			},
			History: history,
		},
	}
}
