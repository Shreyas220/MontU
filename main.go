package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/mux"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type PermissionsTable struct {
	Subject []Subject `json:"subject"`
}
type Subject struct {
	Name            string            `json:"name"`
	Kind            string            `json:"kind"`
	Namespace       string            `json:"namespace,omitempty"`
	RoleBindingInfo []RoleBindingInfo `json:"RoleBindingInfo"`
	BoundedWorkload []string          `json:"BoundedWorkload,omitempty"`
}

type RoleBindingInfo struct {
	Namespace       string         `json:"namespace"`
	RoleBindingName string         `json:"role_binding_name"`
	Name            string         `json:"rolename"`
	Rules           []RoleRuleInfo `json:"rules"`
}

type RoleRuleInfo struct {
	APIGroups []string `json:"api_groups"`
	Resources []string `json:"resources"`
	Verbs     []string `json:"verbs"`
}

var Maproles map[string]MapRoleRuleInfo

func main() {
	r := mux.NewRouter()
	// Define your routes here
	r.HandleFunc("/", respinse)
	// Start the server
	log.Fatal(http.ListenAndServe(":8080", r))
}

var ServiceAccountToPods map[string][]corev1.Pod

var Labels map[string][]rbacv1.Role
var ClusterLabels map[string][]rbacv1.ClusterRole

func getBoundedWorkload(clientset *kubernetes.Clientset) {
	pods, err := clientset.CoreV1().Pods("").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		panic(err)
	}

	// Create a map to store the pods by service account name.
	for _, pod := range pods.Items {
		serviceAccountName := pod.Spec.ServiceAccountName
		ServiceAccountToPods[serviceAccountName] = append(ServiceAccountToPods[serviceAccountName], pod)
	}

}

func respinse(w http.ResponseWriter, r *http.Request) {

	Maproles = make(map[string]MapRoleRuleInfo)
	ServiceAccountToPods = make(map[string][]corev1.Pod)

	config, _ := rest.InClusterConfig()
	clientset, _ := kubernetes.NewForConfig(config)

	getBoundedWorkload(clientset)

	//get all roles and store it in a global variable
	getClusterRoleInfo(clientset)
	getRoleInfo(clientset)

	//get all rolebinding and store it in the
	permissionstable, _ := getRoleBindingInfo(clientset)
	p := PermissionsTable{
		Subject: permissionstable,
	}

	permissionstable1, _ := getClusterRoleBindingInfo(clientset)
	p.Subject = append(p.Subject, permissionstable1...)

	jsonBytes, err := json.MarshalIndent(p, "", "    ")
	if err != nil {
		fmt.Println(err)
		return
	}

	w.Write(jsonBytes)

}

type MapRoleRuleInfo struct {
	Name string
	Rule []RoleRuleInfo
}

func getRoleInfo(clientset *kubernetes.Clientset) error {
	roles, err := clientset.RbacV1().Roles("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, role := range roles.Items {
		for _, label := range role.ObjectMeta.Labels {
			Labels[label] = append(Labels[label], role)
		}
	}
	for _, role := range roles.Items {
		tempmap := MapRoleRuleInfo{}
		tempmap.Name = role.Name
		for _, rule := range role.Rules {
			tempmap.Rule = append(tempmap.Rule, ruleToRoleRuleInfo(rule))
		}
		Maproles[role.Name] = tempmap
	}
	return nil
}

func getClusterRoleInfo(clientset *kubernetes.Clientset) error {
	roles, err := clientset.RbacV1().ClusterRoles().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, role := range roles.Items {
		tempmap := MapRoleRuleInfo{}
		tempmap.Name = role.Name
		for _, rule := range role.Rules {
			tempmap.Rule = append(tempmap.Rule, ruleToRoleRuleInfo(rule))
		}
		if role.AggregationRule != nil {
			for _, label := range role.AggregationRule.ClusterRoleSelectors {
				for key, value := range label.MatchLabels {
					fmt.Println("key: ", key, " value: ", value)
				}
			}
		}
		Maproles[role.Name] = tempmap
	}
	return nil
}

func ruleToRoleRuleInfo(rule rbacv1.PolicyRule) RoleRuleInfo {
	return RoleRuleInfo{
		APIGroups: rule.APIGroups,
		Verbs:     rule.Verbs,
		Resources: rule.Resources,
	}
}

func getRoleBindingInfo(clientset *kubernetes.Clientset) ([]Subject, error) {
	roleBindings, err := clientset.RbacV1().RoleBindings("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var subjects []Subject

	for _, roleBinding := range roleBindings.Items {

		rolebinding := RoleBindingInfo{}
		rolebinding.RoleBindingName = roleBinding.Name
		rolebinding.Namespace = roleBinding.Namespace
		rolebinding.Name = roleBinding.RoleRef.Name

		//check for role in map and loop through to update
		if _, ok := Maproles[roleBinding.RoleRef.Name]; ok {
			rolebinding.Rules = append(rolebinding.Rules, Maproles[roleBinding.RoleRef.Name].Rule...)
		}

		for _, subject := range roleBinding.Subjects {
			subjects = updateOrCreateSubject(subjects, subject, rolebinding)
		}

	}

	return subjects, nil
}

func getClusterRoleBindingInfo(clientset *kubernetes.Clientset) ([]Subject, error) {
	roleBindings, err := clientset.RbacV1().ClusterRoleBindings().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var subjects []Subject

	for _, roleBinding := range roleBindings.Items {

		rolebinding := RoleBindingInfo{}
		rolebinding.RoleBindingName = roleBinding.Name
		rolebinding.Namespace = roleBinding.Namespace
		rolebinding.Name = roleBinding.RoleRef.Name

		//check for role in map and loop through to update
		if _, ok := Maproles[roleBinding.RoleRef.Name]; ok {
			rolebinding.Rules = append(rolebinding.Rules, Maproles[roleBinding.RoleRef.Name].Rule...)
		}

		for _, subject := range roleBinding.Subjects {
			subjects = updateOrCreateSubject(subjects, subject, rolebinding)
		}
	}

	return subjects, nil
}

func updateOrCreateSubject(subjects []Subject, subject rbacv1.Subject, rolebinding RoleBindingInfo) []Subject {
	subjectExists := false
	for i := range subjects {
		if subjects[i].Name == subject.Name && subjects[i].Kind == subject.Kind && subjects[i].Namespace == subject.Namespace {
			subjects[i].RoleBindingInfo = append(subjects[i].RoleBindingInfo, rolebinding)
			subjectExists = true
			break
		}
	}
	if !subjectExists {
		if subject.Kind == "ServiceAccount" {
			Pods, ok := ServiceAccountToPods[subject.Name]
			if ok {
				fmt.Println("Pods related to the service account 'network-team':")
				var boundedWorkload []string
				for _, pod := range Pods {
					boundedWorkload = append(boundedWorkload, pod.Name)
				}
				newSubject := Subject{
					Name:            subject.Name,
					Kind:            subject.Kind,
					Namespace:       subject.Namespace,
					RoleBindingInfo: []RoleBindingInfo{rolebinding},
					BoundedWorkload: boundedWorkload,
				}
				subjects = append(subjects, newSubject)
			} else {
				newSubject := Subject{
					Name:            subject.Name,
					Kind:            subject.Kind,
					Namespace:       subject.Namespace,
					RoleBindingInfo: []RoleBindingInfo{rolebinding},
				}
				subjects = append(subjects, newSubject)
			}

		} else {
			newSubject := Subject{
				Name:            subject.Name,
				Kind:            subject.Kind,
				Namespace:       subject.Namespace,
				RoleBindingInfo: []RoleBindingInfo{rolebinding},
			}
			subjects = append(subjects, newSubject)
		}

	}
	return subjects
}
