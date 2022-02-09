/*
Copyright 2022.

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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	webappv1 "k8ex/api/v1"
)

// RedisReconciler reconciles a Redis object
type RedisReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *RedisReconciler) leaderDeployment(redis webappv1.Redis) (appsv1.Deployment, error) {
	defOne := int32(1)
	depl := appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: appsv1.SchemeGroupVersion.String(), Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      redis.Name + "-leader",
			Namespace: redis.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &defOne,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"redis": redis.Name, "role": "leader"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"redis": redis.Name, "role": "leader"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "leader",
							Image: redis.Spec.LeaderImage,
							Ports: []corev1.ContainerPort{
								{ContainerPort: 6379, Name: "redis", Protocol: "TCP"},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    *resource.NewMilliQuantity(100, resource.DecimalSI),
									corev1.ResourceMemory: *resource.NewMilliQuantity(100000, resource.BinarySI),
								},
							},
						},
					},
				},
			},
		},
	}

	if err := ctrl.SetControllerReference(&redis, &depl, r.Scheme); err != nil {
		return depl, err
	}

	return depl, nil
}

func (r *RedisReconciler) followerDeployment(redis webappv1.Redis) (appsv1.Deployment, error) {
	depl := appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: appsv1.SchemeGroupVersion.String(), Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      redis.Name + "-follower",
			Namespace: redis.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: redis.Spec.FollowerReplicas, // won't be nil because defaulting
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"redis": redis.Name, "role": "follower"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"redis": redis.Name, "role": "follower"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "follower",
							Image: redis.Spec.FollowerImage,
							Ports: []corev1.ContainerPort{
								{ContainerPort: 6379, Name: "redis", Protocol: "TCP"},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    *resource.NewMilliQuantity(100, resource.DecimalSI),
									corev1.ResourceMemory: *resource.NewMilliQuantity(100000, resource.BinarySI),
								},
							},

							// NB(directxman12): sorry about these environment
							// variable names -- they're what the official
							// sample uses (since they used to be the official
							// terms for Redis).  We should change them now that Redis
							// has changed as well.
							Env: []corev1.EnvVar{
								{Name: "GET_HOSTS_FROM", Value: "env"},
								{Name: "REDIS_MASTER_SERVICE_HOST", Value: redis.Name + "-leader"},
								{Name: "REDIS_SLAVE_SERVICE_HOST", Value: redis.Name + "-follower"},
							},
						},
					},
				},
			},
		},
	}

	if err := ctrl.SetControllerReference(&redis, &depl, r.Scheme); err != nil {
		return depl, err
	}

	return depl, nil
}

func (r *RedisReconciler) desiredService(redis webappv1.Redis, role string) (corev1.Service, error) {
	svc := corev1.Service{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String(), Kind: "Service"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      redis.Name + "-" + role,
			Namespace: redis.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "redis", Port: 6379, Protocol: "TCP", TargetPort: intstr.FromString("redis")},
			},
			Selector: map[string]string{"redis": redis.Name, "role": role},
		},
	}

	// always set the controller reference so that we know which object owns this.
	if err := ctrl.SetControllerReference(&redis, &svc, r.Scheme); err != nil {
		return svc, err
	}

	return svc, nil
}

//+kubebuilder:rbac:groups=webapp.example.com,resources=redis,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=webapp.example.com,resources=redis/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=webapp.example.com,resources=redis/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Redis object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *RedisReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("redis", req.NamespacedName)

	log.Info("reconciling redis")

	var redis webappv1.Redis
	if err := r.Get(ctx, req.NamespacedName, &redis); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	leaderDepl, err := r.leaderDeployment(redis)
	if err != nil {
		return ctrl.Result{}, err
	}
	leaderSvc, err := r.desiredService(redis, "leader")
	if err != nil {
		return ctrl.Result{}, err
	}

	followerDepl, err := r.followerDeployment(redis)
	if err != nil {
		return ctrl.Result{}, err
	}
	followerSvc, err := r.desiredService(redis, "follower")
	if err != nil {
		return ctrl.Result{}, err
	}

	applyOpts := []client.PatchOption{client.ForceOwnership, client.FieldOwner("redis-controller")}

	err = r.Patch(ctx, &leaderDepl, client.Apply, applyOpts...)
	if err != nil {
		return ctrl.Result{}, err
	}
	err = r.Patch(ctx, &leaderSvc, client.Apply, applyOpts...)
	if err != nil {
		return ctrl.Result{}, err
	}
	err = r.Patch(ctx, &followerDepl, client.Apply, applyOpts...)
	if err != nil {
		return ctrl.Result{}, err
	}
	err = r.Patch(ctx, &followerSvc, client.Apply, applyOpts...)
	if err != nil {
		return ctrl.Result{}, err
	}

	redis.Status.LeaderService = leaderSvc.Name
	redis.Status.FollowerService = followerSvc.Name

	if err := r.Status().Update(ctx, &redis); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("reconciled redis")

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RedisReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&webappv1.Redis{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
