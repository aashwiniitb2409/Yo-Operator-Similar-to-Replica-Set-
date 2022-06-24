/*
Copyright 2022 Aashwin.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aashwinv1alpha1 "yo/api/v1alpha1"
)

// YoReconciler reconciles a Yo object
type YoReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=aashwin.yo.com,resources=yoes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=aashwin.yo.com,resources=yoes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=aashwin.yo.com,resources=yoes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Yo object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.2/pkg/reconcile
func (r *YoReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// TODO(user): your logic here
	yoinstance := &aashwinv1alpha1.Yo{}
	err := r.Client.Get(context.TODO(), req.NamespacedName, yoinstance)
	if err != nil {
		return ctrl.Result{}, err
	}

	PodList := &corev1.PodList{}

	LabelSelectorSet := map[string]string{
		"app":     yoinstance.Name,
		"version": "v0.1",
	}
	LabelSelector := labels.SelectorFromSet(LabelSelectorSet)

	listOptions := &client.ListOptions{LabelSelector: LabelSelector, Namespace: yoinstance.Namespace}

	if err = r.Client.List(context.TODO(), PodList, listOptions); err != nil {
		return ctrl.Result{}, err
	}

	var availablePods []corev1.Pod
	for _, pod := range PodList.Items {
		if pod.ObjectMeta.DeletionTimestamp != nil {
			continue
		}
		if pod.Status.Phase == corev1.PodRunning || pod.Status.Phase == corev1.PodPending {
			availablePods = append(availablePods, pod)
		}
	}
	log.Log.Info("list of pods", "pods", int32(len(availablePods)))
	currentAvailablePods := int32(len(availablePods))
	availablePodNames := []string{}

	for _, pod := range availablePods {
		availablePodNames = append(availablePodNames, pod.ObjectMeta.Name)
	}

	status := aashwinv1alpha1.YoStatus{
		Podname: availablePodNames,
	}

	if !reflect.DeepEqual(yoinstance.Status, status) {
		yoinstance.Status = status
		err = r.Client.Status().Update(context.TODO(), yoinstance)
		if err != nil {
			log.Log.Error(err, "Failed to update status of PodSet")
			return ctrl.Result{}, err
		}
	}

	if currentAvailablePods > yoinstance.Spec.Replicas {
		log.Log.Info("Scaling down PodSet", "currently available", currentAvailablePods, "required", yoinstance.Spec.Replicas)
		difference := currentAvailablePods - yoinstance.Spec.Replicas
		podsToBeDestroyed := availablePods[:difference]
		for _, soonToBeDestroyedPod := range podsToBeDestroyed {
			err = r.Client.Delete(context.TODO(), &soonToBeDestroyedPod)
			log.Log.Error(err, "Failed to delete Pod from PodSet", "PodName", soonToBeDestroyedPod.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	if currentAvailablePods < yoinstance.Spec.Replicas {
		log.Log.Info("Scaling up PodSet", "currently available", currentAvailablePods, "required", yoinstance.Spec.Replicas)
		pod := newPodForPodSetCustomResource(yoinstance)

		if err = controllerutil.SetControllerReference(yoinstance, pod, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		err = r.Client.Create(context.TODO(), pod)
		if err != nil {
			log.Log.Error(err, "Failed to create a new Pod for the PodSet custom resource")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *YoReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&aashwinv1alpha1.Yo{}).
		Owns(&corev1.Pod{}).
		Complete(r)
}

func newPodForPodSetCustomResource(cr *aashwinv1alpha1.Yo) *corev1.Pod {
	labelsForNewPod := map[string]string{
		"app":     cr.Name,
		"version": "v0.1",
	}

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: cr.Name + "-pod-",
			Namespace:    cr.Namespace,
			Labels:       labelsForNewPod,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "nginx",
					Image:   "nginx:latest",
					Command: []string{"sleep", "3600"},
				},
			},
		},
	}
}
