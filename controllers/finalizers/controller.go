// Copyright 2021 Chaos Mesh Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package finalizers

import (
	"context"

	"k8s.io/client-go/tools/record"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/chaos-mesh/chaos-mesh/api/v1alpha1"
)

const (
	// AnnotationCleanFinalizer key
	AnnotationCleanFinalizer = `chaos-mesh.chaos-mesh.org/cleanFinalizer`
	// AnnotationCleanFinalizerForced value
	AnnotationCleanFinalizerForced = `forced`
)

// Reconciler for common chaos
type Reconciler struct {
	// Object is used to mark the target type of this Reconciler
	Object v1alpha1.InnerObject

	// Client is used to operate on the Kubernetes cluster
	client.Client
	client.Reader

	Recorder record.EventRecorder

	Log logr.Logger
}

// Reconcile the common chaos
func (r *Reconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	obj := r.Object.DeepCopyObject().(v1alpha1.InnerObject)

	if err := r.Client.Get(context.TODO(), req.NamespacedName, obj); err != nil {
		if apierrors.IsNotFound(err) {
			r.Log.Info("chaos not found")
		} else {
			// TODO: handle this error
			r.Log.Error(err, "unable to get chaos")
		}
		return ctrl.Result{}, nil
	}

	finalizers := obj.GetObjectMeta().Finalizers
	records := obj.GetStatus().Experiment.Records
	shouldUpdate := false
	if obj.IsDeleted() {
		resumed := true
		for _, record := range records {
			if record.Phase != v1alpha1.NotInjected {
				resumed = false
			}
		}

		if obj.GetObjectMeta().Annotations[AnnotationCleanFinalizer] == AnnotationCleanFinalizerForced || (resumed && len(finalizers) != 0) {
			r.Recorder.Event(obj, "Normal", "AllRecovered", "All records are recovered")
			finalizers = []string{}
			shouldUpdate = true
		}
	} else {
		if len(finalizers) == 0 || finalizers[0] != "chaos-mesh/records" {
			r.Recorder.Event(obj, "Normal", "Created", "Add finalizer \"chaos-mesh/records\"")
			shouldUpdate = true
			finalizers = []string{"chaos-mesh/records"}
		}
	}

	if shouldUpdate {
		updateError := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			obj := r.Object.DeepCopyObject().(v1alpha1.InnerObject)

			if err := r.Client.Get(context.TODO(), req.NamespacedName, obj); err != nil {
				r.Log.Error(err, "unable to get chaos")
				return err
			}

			obj.GetObjectMeta().Finalizers = finalizers
			return r.Client.Update(context.TODO(), obj)
		})
		if updateError != nil {
			// TODO: handle this error
			r.Log.Error(updateError, "fail to update")
			r.Recorder.Eventf(obj, "Normal", "Failed", "Failed to update finalizer: %s", updateError.Error())
			return ctrl.Result{}, nil
		}

		r.Recorder.Event(obj, "Normal", "Updated", "Successfully update finalizer of resource")
	}
	return ctrl.Result{}, nil
}
