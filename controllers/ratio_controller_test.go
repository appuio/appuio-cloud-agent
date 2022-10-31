package controllers

import (
	"context"
	"errors"

	"github.com/appuio/appuio-cloud-agent/ratio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"testing"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestRatioReconciler_Warn(t *testing.T) {
	recorder := record.NewFakeRecorder(4)
	_, err := prepareRatioTest(t, testRatioCfg{
		limit:       resource.MustParse("4G"),
		fetchMemory: resource.MustParse("4G"),
		fetchCPU:    resource.MustParse("1100m"),
		recorder:    recorder,
		obj: []client.Object{
			testNs,
			testPod,
		},
	}).Reconcile(context.TODO(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNs.Name,
			Name:      testPod.Name,
		},
	})
	assert.NoError(t, err)
	require.Len(t, recorder.Events, 2)
}

func TestRatioReconciler_Ok(t *testing.T) {
	recorder := record.NewFakeRecorder(4)
	_, err := prepareRatioTest(t, testRatioCfg{
		limit:       resource.MustParse("4G"),
		fetchMemory: resource.MustParse("4G"),
		fetchCPU:    resource.MustParse("900m"),
		recorder:    recorder,
		obj: []client.Object{
			testNs,
			testPod,
		},
	}).Reconcile(context.TODO(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNs.Name,
			Name:      testPod.Name,
		},
	})
	assert.NoError(t, err)
	require.Len(t, recorder.Events, 0)
}

func TestRatioReconciler_Disabled(t *testing.T) {
	recorder := record.NewFakeRecorder(4)
	_, err := prepareRatioTest(t, testRatioCfg{
		limit:    resource.MustParse("4G"),
		fetchErr: ratio.ErrorDisabled,
		recorder: recorder,
		obj: []client.Object{
			testNs,
			testPod,
		},
	}).Reconcile(context.TODO(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNs.Name,
			Name:      testPod.Name,
		},
	})
	assert.NoError(t, err)
	require.Len(t, recorder.Events, 0)
}

func TestRatioReconciler_Failed(t *testing.T) {
	recorder := record.NewFakeRecorder(4)
	_, err := prepareRatioTest(t, testRatioCfg{
		limit:    resource.MustParse("4G"),
		fetchErr: errors.New("internal"),
		recorder: recorder,
		obj: []client.Object{
			testNs,
			testPod,
		},
	}).Reconcile(context.TODO(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNs.Name,
			Name:      testPod.Name,
		},
	})
	assert.Error(t, err)
	require.Len(t, recorder.Events, 0)
}

func TestRatioReconciler_RecordFailed(t *testing.T) {
	wrongNs := *testNs
	wrongNs.Name = "bar"
	wrongPod := *testPod
	wrongPod.Name = "asf"
	wrongPod.Namespace = "asf"
	recorder := record.NewFakeRecorder(4)
	_, err := prepareRatioTest(t, testRatioCfg{
		limit:       resource.MustParse("4G"),
		fetchMemory: resource.MustParse("4G"),
		fetchCPU:    resource.MustParse("1100m"),
		recorder:    recorder,
		obj: []client.Object{
			&wrongNs,
			&wrongPod,
		},
	}).Reconcile(context.TODO(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNs.Name,
			Name:      testPod.Name,
		},
	})
	assert.NoError(t, err)
	if !assert.Len(t, recorder.Events, 0) {
		for i := 0; i < len(recorder.Events); i++ {
			e := <-recorder.Events
			t.Log(e)
		}
	}
}

type testRatioCfg struct {
	limit       resource.Quantity
	fetchErr    error
	fetchCPU    resource.Quantity
	fetchMemory resource.Quantity
	obj         []client.Object
	recorder    record.EventRecorder
}

func prepareRatioTest(t *testing.T, cfg testRatioCfg) *RatioReconciler {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cfg.obj...).
		Build()

	if cfg.recorder == nil {
		cfg.recorder = &record.FakeRecorder{}
	}

	return &RatioReconciler{
		Client:   client,
		Recorder: cfg.recorder,
		Scheme:   scheme,
		Ratio: fakeRatioFetcher{
			err: cfg.fetchErr,
			ratio: &ratio.Ratio{
				CPU:    cfg.fetchCPU.AsDec(),
				Memory: cfg.fetchMemory.AsDec(),
			},
		},
		RatioLimit: &cfg.limit,
	}
}

type fakeRatioFetcher struct {
	err   error
	ratio *ratio.Ratio
}

func (f fakeRatioFetcher) FetchRatio(ctx context.Context, ns string) (*ratio.Ratio, error) {
	return f.ratio, f.err
}

var testNs = &corev1.Namespace{
	ObjectMeta: metav1.ObjectMeta{
		Name: "foo",
	},
}
var testPod = &corev1.Pod{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "pod",
		Namespace: "foo",
	},
	Status: corev1.PodStatus{
		Phase: corev1.PodRunning,
	},
}
