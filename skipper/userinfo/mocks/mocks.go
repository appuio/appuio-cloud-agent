package mocks

import (
	"k8s.io/apimachinery/pkg/labels"

	rbacv1 "k8s.io/api/rbac/v1"
	rbacv1listers "k8s.io/client-go/listers/rbac/v1"
)

type MockLister[T any] struct {
	Objects []T
}

func (l *MockLister[T]) List(_ labels.Selector) ([]T, error) {
	return l.Objects, nil
}

func (l *MockLister[T]) Get(_ string) (T, error) {
	return l.Objects[0], nil
}

type MockRoleBindingLister struct {
	MockLister[*rbacv1.RoleBinding]
}

func (l *MockRoleBindingLister) RoleBindings(_ string) rbacv1listers.RoleBindingNamespaceLister {
	return &l.MockLister
}
