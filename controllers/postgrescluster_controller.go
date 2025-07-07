package controllers

import (
    "context"
    "fmt"
    "time"

    appsv1 "k8s.io/api/apps/v1"
    corev1 "k8s.io/api/core/v1"
    "k8s.io/apimachinery/pkg/api/errors"
    "k8s.io/apimachinery/pkg/api/resource"
    "k8s.io/apimachinery/pkg/runtime"   // <-- add this line
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/types"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
    "sigs.k8s.io/controller-runtime/pkg/log"

    databasev1 "postgres-operator/api/v1"
    "postgres-operator/internal/k8s"
    "postgres-operator/internal/utils"
)

// PostgresClusterReconciler reconciles a PostgresCluster object
type PostgresClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=database.example.com,resources=postgresclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=database.example.com,resources=postgresclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=database.example.com,resources=postgresclusters/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

func (r *PostgresClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the PostgresCluster instance
	cluster := &databasev1.PostgresCluster{}
	err := r.Get(ctx, req.NamespacedName, cluster)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("PostgresCluster resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get PostgresCluster")
		return ctrl.Result{}, err
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(cluster, "database.example.com/postgres-cluster") {
		controllerutil.AddFinalizer(cluster, "database.example.com/postgres-cluster")
		return ctrl.Result{}, r.Update(ctx, cluster)
	}

	// Handle deletion
	if cluster.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, cluster)
	}

	// Set default values
	r.setDefaults(cluster)

	// Reconcile the PostgreSQL cluster
	if err := r.reconcileCluster(ctx, cluster); err != nil {
		logger.Error(err, "Failed to reconcile PostgreSQL cluster")
		return ctrl.Result{}, err
	}

	// Update status
	if err := r.updateStatus(ctx, cluster); err != nil {
		logger.Error(err, "Failed to update PostgresCluster status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
}

func (r *PostgresClusterReconciler) setDefaults(cluster *databasev1.PostgresCluster) {
	if cluster.Spec.Replicas == 0 {
		cluster.Spec.Replicas = 1
	}
	if cluster.Spec.PostgresVersion == "" {
		cluster.Spec.PostgresVersion = "15"
	}
	if cluster.Spec.Database.Name == "" {
		cluster.Spec.Database.Name = "postgres"
	}
	if cluster.Spec.Storage.Size == "" {
		cluster.Spec.Storage.Size = "10Gi"
	}
	if cluster.Spec.Resources.CPU == "" {
		cluster.Spec.Resources.CPU = "500m"
	}
	if cluster.Spec.Resources.Memory == "" {
		cluster.Spec.Resources.Memory = "1Gi"
	}
}

func (r *PostgresClusterReconciler) reconcileCluster(ctx context.Context, cluster *databasev1.PostgresCluster) error {
	logger := log.FromContext(ctx)

	// Create Secret for PostgreSQL credentials
	if err := r.reconcileSecret(ctx, cluster); err != nil {
		return fmt.Errorf("failed to reconcile Secret: %w", err)
	}

	// Create Service for PostgreSQL
	if err := r.reconcileService(ctx, cluster); err != nil {
		return fmt.Errorf("failed to reconcile Service: %w", err)
	}

	// Create StatefulSet for PostgreSQL
	if err := r.reconcileStatefulSet(ctx, cluster); err != nil {
		return fmt.Errorf("failed to reconcile StatefulSet: %w", err)
	}

	logger.Info("Successfully reconciled PostgreSQL cluster", "cluster", cluster.Name)
	return nil
}

func (r *PostgresClusterReconciler) reconcileSecret(ctx context.Context, cluster *databasev1.PostgresCluster) error {
	// Generate secure passwords
	postgresPassword, err := utils.GeneratePassword(16)
	if err != nil {
		return fmt.Errorf("failed to generate postgres password: %w", err)
	}

	replicationPassword, err := utils.GeneratePassword(16)
	if err != nil {
		return fmt.Errorf("failed to generate replication password: %w", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name + "-credentials",
			Namespace: cluster.Namespace,
		},
		Data: map[string][]byte{
			"postgres-password":       []byte(postgresPassword),
			"replication-password":    []byte(replicationPassword),
		},
	}

	if err := controllerutil.SetControllerReference(cluster, secret, r.Scheme); err != nil {
		return err
	}

	return k8s.CreateOrUpdate(ctx, r.Client, secret)
}

func (r *PostgresClusterReconciler) reconcileService(ctx context.Context, cluster *databasev1.PostgresCluster) error {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name + "-service",
			Namespace: cluster.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app":     "postgres",
				"cluster": cluster.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:     "postgres",
					Port:     5432,
					Protocol: corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	if err := controllerutil.SetControllerReference(cluster, service, r.Scheme); err != nil {
		return err
	}

	return k8s.CreateOrUpdate(ctx, r.Client, service)
}

func (r *PostgresClusterReconciler) reconcileStatefulSet(ctx context.Context, cluster *databasev1.PostgresCluster) error {
	logger := log.FromContext(ctx)

	// Parse resource requirements
	cpuRequest, err := resource.ParseQuantity(cluster.Spec.Resources.CPU)
	if err != nil {
		return fmt.Errorf("invalid CPU value: %w", err)
	}
	memRequest, err := resource.ParseQuantity(cluster.Spec.Resources.Memory)
	if err != nil {
		return fmt.Errorf("invalid Memory value: %w", err)
	}

	// Set access modes (default to ReadWriteOnce)
	accessModes := []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
	if len(cluster.Spec.Storage.AccessModes) > 0 {
		accessModes = make([]corev1.PersistentVolumeAccessMode, len(cluster.Spec.Storage.AccessModes))
		for i, mode := range cluster.Spec.Storage.AccessModes {
			accessModes[i] = corev1.PersistentVolumeAccessMode(mode)
		}
	}

	// Configure storage class (nil means use default)
	var storageClassName *string
	if cluster.Spec.Storage.StorageClass != "" {
		storageClassName = &cluster.Spec.Storage.StorageClass
	}

	// Configure security context
	runAsUser := int64(999) // postgres user
	fsGroup := int64(999)   // postgres group

	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name,
			Namespace: cluster.Namespace,
			Labels: map[string]string{
				"app":     "postgres",
				"cluster": cluster.Name,
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &cluster.Spec.Replicas,
			ServiceName: fmt.Sprintf("%s-service", cluster.Name),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":     "postgres",
					"cluster": cluster.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":     "postgres",
						"cluster": cluster.Name,
					},
				},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsUser: &runAsUser,
						FSGroup:   &fsGroup,
					},
					Containers: []corev1.Container{
						{
							Name:  "postgres",
							Image: fmt.Sprintf("postgres:%s", cluster.Spec.PostgresVersion),
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 5432,
									Name:          "postgres",
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "POSTGRES_DB",
									Value: cluster.Spec.Database.Name,
								},
								{
									Name: "POSTGRES_PASSWORD",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: cluster.Name + "-credentials",
											},
											Key: "postgres-password",
										},
									},
								},
								{
									Name: "POSTGRES_REPLICATION_PASSWORD",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: cluster.Name + "-credentials",
											},
											Key: "replication-password",
										},
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "data",
									MountPath: "/var/lib/postgresql/data",
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    cpuRequest,
									corev1.ResourceMemory: memRequest,
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    cpuRequest,
									corev1.ResourceMemory: memRequest,
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{
											"pg_isready",
											"-U", "postgres",
											"-h", "localhost",
											"-p", "5432",
										},
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
								TimeoutSeconds:      5,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{
											"pg_isready",
											"-U", "postgres",
											"-h", "localhost",
											"-p", "5432",
										},
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       5,
								TimeoutSeconds:      1,
							},
						},
					},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "data",
						Labels: map[string]string{
							"app":     "postgres",
							"cluster": cluster.Name,
						},
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: accessModes,
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse(cluster.Spec.Storage.Size),
							},
						},
						StorageClassName: storageClassName,
					},
				},
			},
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(cluster, statefulSet, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	// Create or update StatefulSet
	if err := k8s.CreateOrUpdate(ctx, r.Client, statefulSet); err != nil {
		return fmt.Errorf("failed to create/update StatefulSet: %w", err)
	}

	logger.Info("Successfully reconciled StatefulSet", "statefulSet", statefulSet.Name)
	return nil
}

func (r *PostgresClusterReconciler) updateStatus(ctx context.Context, cluster *databasev1.PostgresCluster) error {
	// Get StatefulSet to check status
	statefulSet := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, statefulSet)
	if err != nil {
		cluster.Status.Phase = "Failed"
		cluster.Status.Message = fmt.Sprintf("Failed to get StatefulSet: %v", err)
		return r.Status().Update(ctx, cluster)
	}

	// Update status based on StatefulSet status
	cluster.Status.ReadyReplicas = statefulSet.Status.ReadyReplicas
	
	if statefulSet.Status.ReadyReplicas == cluster.Spec.Replicas {
		cluster.Status.Phase = "Running"
		cluster.Status.Message = "All replicas are ready"
	} else {
		cluster.Status.Phase = "Pending"
		cluster.Status.Message = fmt.Sprintf("Waiting for replicas: %d/%d ready", statefulSet.Status.ReadyReplicas, cluster.Spec.Replicas)
	}

	// Set current primary (for simplicity, assume first pod is primary)
	if statefulSet.Status.ReadyReplicas > 0 {
		cluster.Status.CurrentPrimary = fmt.Sprintf("%s-0", cluster.Name)
	}

	cluster.Status.DatabaseVersion = cluster.Spec.PostgresVersion

	return r.Status().Update(ctx, cluster)
}

func (r *PostgresClusterReconciler) handleDeletion(ctx context.Context, cluster *databasev1.PostgresCluster) (ctrl.Result, error) {
    logger := log.FromContext(ctx)
    logger.Info("Handling deletion of PostgresCluster", "cluster", cluster.Name)

    controllerutil.RemoveFinalizer(cluster, "database.example.com/postgres-cluster")
    if err := r.Update(ctx, cluster); err != nil {
        return ctrl.Result{}, err
    }
    return ctrl.Result{}, nil
}

func (r *PostgresClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&databasev1.PostgresCluster{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}