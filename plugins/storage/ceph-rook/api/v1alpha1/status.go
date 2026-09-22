package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// These accessors let the plugin's shared status writer drive both status
// types through one generic implementation.

func (s *ConsumerStatus) SetObservedGeneration(g int64)      { s.ObservedGeneration = g }
func (s *ConsumerStatus) ConditionsRef() *[]metav1.Condition { return &s.Conditions }
func (s *ConsumerStatus) SetDegraded(message string)         { s.Phase, s.Message = PhaseDegraded, message }

func (s *DiskPoolStatus) SetObservedGeneration(g int64)      { s.ObservedGeneration = g }
func (s *DiskPoolStatus) ConditionsRef() *[]metav1.Condition { return &s.Conditions }
func (s *DiskPoolStatus) SetDegraded(message string)         { s.Phase, s.Message = PhaseDegraded, message }
