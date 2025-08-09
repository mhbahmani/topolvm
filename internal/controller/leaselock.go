package controller

import (
	"context"
	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"
	"time"
)

type LeaseLock struct {
	name      string
	namespace string
	holderID  string
	client    client.Client
}

func NewLeaseLock(name string, namespace string, k8sClient client.Client, holder string) *LeaseLock {
	return &LeaseLock{
		name:      name,
		namespace: namespace,
		holderID:  holder,
		client:    k8sClient,
	}
}

func (l *LeaseLock) AcquireLeaseLock(ctx context.Context) bool {
	log := crlog.FromContext(ctx)

	log.Info("Trying to acquire lease lock '%s/%s' with holderID ID '%s'.")
	newLease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      l.name,
			Namespace: l.namespace,
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity:       &l.holderID,
			LeaseDurationSeconds: func() *int32 { i := int32(20); return &i }(),
			AcquireTime:          &metav1.MicroTime{Time: time.Now()},
			RenewTime:            &metav1.MicroTime{Time: time.Now()},
		},
	}

	// Attempt to create the lease directly.
	err := l.client.Create(ctx, newLease)
	if err == nil {
		log.Info("Successfully created lease. LeaseLock acquired by '%s'.", l.holderID)
		return true
	}

	if apierrors.IsAlreadyExists(err) {
		log.Info("Lease already exists. Could not acquire lock.")
		return true
	}
	log.Error(err, "Failed to create lease")
	return false
}

func (l *LeaseLock) ReleaseLeaseLock(ctx context.Context) {
	log := crlog.FromContext(ctx)

	lease := &coordinationv1.Lease{}
	err := l.client.Get(ctx, client.ObjectKey{Namespace: l.namespace, Name: l.name}, lease)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Lease '%s/%s' not found. LeaseLock is already released.", l.namespace, l.name)
			return
		}
		log.Error(err, "Failed to get lease '%s/%s' for releasing.", l.namespace, l.name)
		return
	}

	if lease.Spec.HolderIdentity == nil || *lease.Spec.HolderIdentity != l.holderID {
		log.Error(nil, "Lease %s/%s is not held by '%s'. Cannot release it.", l.namespace, l.name, l.holderID)
		return
	}

	if err := l.client.Delete(ctx, lease); err != nil {
		if apierrors.IsNotFound(err) {
			return
		}
		log.Error(err, "Failed to delete lease '%s/%s' held by '%s'.", l.namespace, l.name, l.holderID)
		return
	}

	log.Info("Successfully released lease '%s/%s' held by '%s'.", l.namespace, l.name, l.name)
}
