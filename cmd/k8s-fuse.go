package cmd

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"path/filepath"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/spf13/cobra"

	// appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	// "k8s.io/client-go/util/retry"
)

/*
Lots of duplication but the client-go package is not great with being
able to use Generics.

Will figure that out, a lot of it is just reading the particular API
then marshaling the result into a file with the same name as the
resource.
*/
type KubernetesRoot struct {
	fs.Inode
	clientset *kubernetes.Clientset
}

type KubernetesNamespace struct {
	fs.Inode
	clientset *kubernetes.Clientset
	namespace apiv1.Namespace
}

type KubernetesServices struct {
	fs.Inode
	clientset *kubernetes.Clientset
	namespace string
}

type KubernetesDeployments struct {
	fs.Inode
	clientset *kubernetes.Clientset
	namespace string
}

type KubernetesIngress struct {
	fs.Inode
	clientset *kubernetes.Clientset
	namespace string
}

func (r *KubernetesServices) OnAdd(ctx context.Context) {
	services, err := r.clientset.CoreV1().Services(r.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		panic(err)
	}

	for _, resource := range services.Items {
		b, err := json.MarshalIndent(resource, "", " ")
		if err != nil {
			panic(err)
		}
		ch := r.NewPersistentInode(
			ctx, &fs.MemRegularFile{
				Data: b,
				Attr: fuse.Attr{
					Mode: 0644,
				},
			}, fs.StableAttr{})
		r.AddChild(resource.Name, ch, false)
	}
}

func (r *KubernetesDeployments) OnAdd(ctx context.Context) {
	deployments, err := r.clientset.AppsV1().Deployments(r.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		panic(err)
	}

	for _, resource := range deployments.Items {
		b, err := json.MarshalIndent(resource, "", " ")
		if err != nil {
			panic(err)
		}
		ch := r.NewPersistentInode(
			ctx, &fs.MemRegularFile{
				Data: b,
				Attr: fuse.Attr{
					Mode: 0644,
				},
			}, fs.StableAttr{})
		r.AddChild(resource.Name, ch, false)
	}
}

func (r *KubernetesIngress) OnAdd(ctx context.Context) {
	ingresses, err := r.clientset.NetworkingV1().Ingresses(r.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		panic(err)
	}

	for _, resource := range ingresses.Items {
		b, err := json.MarshalIndent(resource, "", " ")
		if err != nil {
			panic(err)
		}
		ch := r.NewPersistentInode(
			ctx, &fs.MemRegularFile{
				Data: b,
				Attr: fuse.Attr{
					Mode: 0644,
				},
			}, fs.StableAttr{})
		r.AddChild(resource.Name, ch, false)
	}
}

func (r *KubernetesNamespace) OnAdd(ctx context.Context) {
	b, err := json.MarshalIndent(r.namespace, "", " ")
	if err != nil {
		panic(err)
	}
	ch := r.NewPersistentInode(
		ctx, &fs.MemRegularFile{
			Data: b,
			Attr: fuse.Attr{
				Mode: 0644,
			},
		}, fs.StableAttr{})
	r.AddChild("namespace.json", ch, false)

	services := r.NewPersistentInode(
		ctx,
		&KubernetesServices{
			clientset: r.clientset,
			namespace: r.namespace.Name,
		},
		fs.StableAttr{Mode: fuse.S_IFDIR},
	)
	r.AddChild("services", services, false)
	deployments := r.NewPersistentInode(
		ctx,
		&KubernetesDeployments{
			clientset: r.clientset,
			namespace: r.namespace.Name,
		},
		fs.StableAttr{Mode: fuse.S_IFDIR},
	)
	r.AddChild("deployments", deployments, false)
	ingresses := r.NewPersistentInode(
		ctx,
		&KubernetesIngress{
			clientset: r.clientset,
			namespace: r.namespace.Name,
		},
		fs.StableAttr{Mode: fuse.S_IFDIR},
	)
	r.AddChild("ingresses", ingresses, false)
}

func (r *KubernetesRoot) OnAdd(ctx context.Context) {

	if r.clientset == nil {
		var kubeconfig *string
		if home := homedir.HomeDir(); home != "" {
			kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
		} else {
			kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
		}
		flag.Parse()

		config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
		if err != nil {
			panic(err)
		}
		clientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			panic(err)
		}
		r.clientset = clientset
	}

	namespaces, err := r.clientset.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err)
	}

	for _, namespace := range namespaces.Items {
		chDir := r.NewPersistentInode(ctx, &KubernetesNamespace{clientset: r.clientset, namespace: namespace},
			fs.StableAttr{Mode: fuse.S_IFDIR})
		r.AddChild(namespace.Name, chDir, false)
	}

}

func (r *KubernetesRoot) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = 0755
	return 0
}

var mountCmd = &cobra.Command{
	Use: "mount",
	Run: func(cmd *cobra.Command, args []string) {

		opts := &fs.Options{}
		opts.Debug = false
		server, err := fs.Mount(args[0], &KubernetesRoot{}, opts)
		if err != nil {
			log.Fatalf("Mount fail: %v\n", err)
		}
		server.Wait()
	},
}

func init() {
	rootCmd.AddCommand(mountCmd)
}
