package controllers

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	databasev1 "postgres-operator/api/v1"
	"postgres-operator/internal/postgres"
	"postgres-operator/internal/utils"
)

// PostgresUserReconciler reconciles a PostgresUser object
type PostgresUserReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=database.example.com,resources=postgresusers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=database.example.com,resources=postgresusers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=database.example.com,resources=postgresusers/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete

func (r *PostgresUserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the PostgresUser instance
	user := &databasev1.PostgresUser{}
	err := r.Get(ctx, req.NamespacedName, user)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("PostgresUser resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get PostgresUser")
		return ctrl.Result{}, err
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(user, "database.example.com/postgres-user") {
		controllerutil.AddFinalizer(user, "database.example.com/postgres-user")
		return ctrl.Result{}, r.Update(ctx, user)
	}

	// Handle deletion
	if user.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, user)
	}

	// Get the referenced cluster
	cluster := &databasev1.PostgresCluster{}
	clusterKey := types.NamespacedName{
		Name:      user.Spec.ClusterRef.Name,
		Namespace: user.Spec.ClusterRef.Namespace,
	}
	if user.Spec.ClusterRef.Namespace == "" {
		clusterKey.Namespace = user.Namespace
	}

	err = r.Get(ctx, clusterKey, cluster)
	if err != nil {
		logger.Error(err, "Failed to get referenced PostgresCluster")
		user.Status.Phase = "Failed"
		user.Status.Message = fmt.Sprintf("Failed to get cluster %s: %v", user.Spec.ClusterRef.Name, err)
		return ctrl.Result{}, r.Status().Update(ctx, user)
	}

	// Reconcile the user
	if err := r.reconcileUser(ctx, user, cluster); err != nil {
		logger.Error(err, "Failed to reconcile PostgreSQL user")
		user.Status.Phase = "Failed"
		user.Status.Message = fmt.Sprintf("Failed to reconcile user: %v", err)
		r.Status().Update(ctx, user)
		return ctrl.Result{}, err
	}

	// Update status
	user.Status.Phase = "Ready"
	user.Status.Message = "User successfully created/updated"
	user.Status.DatabasesGranted = user.Spec.Databases
	user.Status.LastPasswordChange = &metav1.Time{Time: time.Now()}

	return ctrl.Result{RequeueAfter: time.Minute * 10}, r.Status().Update(ctx, user)
}

func (r *PostgresUserReconciler) reconcileUser(ctx context.Context, user *databasev1.PostgresUser, cluster *databasev1.PostgresCluster) error {
	// Create password secret if needed
	if err := r.reconcilePasswordSecret(ctx, user); err != nil {
		return fmt.Errorf("failed to reconcile password secret: %w", err)
	}

	// Connect to PostgreSQL using internal client
	pgClient, err := postgres.NewClient(ctx, r.Client, cluster)
	if err != nil {
		return fmt.Errorf("failed to create database client: %w", err)
	}
	defer pgClient.Close()

	// Get password from secret
	password, err := r.getPasswordFromSecret(ctx, user)
	if err != nil {
		return fmt.Errorf("failed to get password: %w", err)
	}

	// Create or update user in PostgreSQL
	if err := pgClient.CreateOrUpdateUser(ctx, user.Spec.Username, password, user.Spec.Privileges); err != nil {
		return fmt.Errorf("failed to create/update user in database: %w", err)
	}

	// Grant database access
	for _, dbName := range user.Spec.Databases {
		if err := pgClient.GrantDatabaseAccess(ctx, user.Spec.Username, dbName); err != nil {
			return fmt.Errorf("failed to grant access to database %s: %w", dbName, err)
		}
	}

	return nil
}

func (r *PostgresUserReconciler) reconcilePasswordSecret(ctx context.Context, user *databasev1.PostgresUser) error {
	logger := log.FromContext(ctx)
	
	if user.Spec.Password.SecretRef != nil {
		// Verify the referenced secret exists
		secret := &corev1.Secret{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      user.Spec.Password.SecretRef.Name,
			Namespace: user.Namespace,
		}, secret)
		
		if err != nil {
			if errors.IsNotFound(err) {
				return fmt.Errorf("referenced secret %s not found", user.Spec.Password.SecretRef.Name)
			}
			return fmt.Errorf("failed to get referenced secret: %w", err)
		}
		
		// Verify password key exists in secret
		key := "password"
		if user.Spec.Password.SecretRef.Key != "" {
			key = user.Spec.Password.SecretRef.Key
		}
		
		if _, exists := secret.Data[key]; !exists {
			return fmt.Errorf("secret %s missing required key '%s'", user.Spec.Password.SecretRef.Name, key)
		}
		
		// Everything looks good with the referenced secret
		return nil
	}

	if !user.Spec.Password.Generate {
		return fmt.Errorf("password must be either provided via secretRef or generated")
	}

	// Set default password length if not specified
	length := 16
	if user.Spec.Password.Length > 0 {
		length = int(user.Spec.Password.Length)
	} else {
		logger.Info("Using default password length", "length", length)
	}

	// Generate password using utils
	password, err := utils.GeneratePassword(length)
	if err != nil {
		return fmt.Errorf("failed to generate password: %w", err)
	}

	// Create or update secret
	secretName := fmt.Sprintf("%s-password", user.Name)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: user.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "postgres-user",
				"app.kubernetes.io/instance":   user.Name,
				"app.kubernetes.io/managed-by": "postgres-operator",
			},
		},
		Data: map[string][]byte{
			"password": []byte(password),
		},
	}

	if err := controllerutil.SetControllerReference(user, secret, r.Scheme); err != nil {
		return err
	}

	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		// Only update the password if it's a new secret or if we're explicitly rotating passwords
		if secret.CreationTimestamp.IsZero() {
			secret.Data = map[string][]byte{
				"password": []byte(password),
			}
		}
		return nil
	})
	
	return err
}

func (r *PostgresUserReconciler) getPasswordFromSecret(ctx context.Context, user *databasev1.PostgresUser) (string, error) {
	var secretName, secretKey string

	if user.Spec.Password.SecretRef != nil {
		secretName = user.Spec.Password.SecretRef.Name
		secretKey = user.Spec.Password.SecretRef.Key
		if secretKey == "" {
			secretKey = "password"
		}
	} else {
		secretName = fmt.Sprintf("%s-password", user.Name)
		secretKey = "password"
	}

	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: user.Namespace}, secret)
	if err != nil {
		return "", err
	}

	password, exists := secret.Data[secretKey]
	if !exists {
		return "", fmt.Errorf("password key %s not found in secret %s", secretKey, secretName)
	}

	return string(password), nil
}

func (r *PostgresUserReconciler) handleDeletion(ctx context.Context, user *databasev1.PostgresUser) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Handling deletion of PostgresUser", "user", user.Name)

	// Only attempt to drop user if cluster exists
	if user.Status.Phase == "Ready" {
		cluster := &databasev1.PostgresCluster{}
		clusterKey := types.NamespacedName{
			Name:      user.Spec.ClusterRef.Name,
			Namespace: user.Spec.ClusterRef.Namespace,
		}
		if user.Spec.ClusterRef.Namespace == "" {
			clusterKey.Namespace = user.Namespace
		}

		err := r.Get(ctx, clusterKey, cluster)
		if err == nil {
			pgClient, err := postgres.NewClient(ctx, r.Client, cluster)
			if err == nil {
				defer pgClient.Close()
				if err := pgClient.DropUser(ctx, user.Spec.Username); err != nil {
					logger.Error(err, "Failed to drop user from database")
				}
			} else {
				logger.Error(err, "Failed to create database client during deletion")
			}
		}
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(user, "database.example.com/postgres-user")
	return ctrl.Result{}, r.Update(ctx, user)
}

func (r *PostgresUserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&databasev1.PostgresUser{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}