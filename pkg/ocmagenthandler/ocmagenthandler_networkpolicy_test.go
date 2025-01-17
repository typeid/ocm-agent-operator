package ocmagenthandler

import (
	"context"
	"reflect"

	k8serrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"go.uber.org/mock/gomock"

	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ocmagentv1alpha1 "github.com/openshift/ocm-agent-operator/api/v1alpha1"
	oah "github.com/openshift/ocm-agent-operator/pkg/consts/ocmagenthandler"
	testconst "github.com/openshift/ocm-agent-operator/pkg/consts/test/init"
	clientmocks "github.com/openshift/ocm-agent-operator/pkg/util/test/generated/mocks/client"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("OCM Agent NetworkPolicy Handler", func() {
	var (
		mockClient *clientmocks.MockClient
		mockCtrl   *gomock.Controller

		testOcmAgent        ocmagentv1alpha1.OcmAgent
		testOcmAgentHandler ocmAgentHandler
		testHSOcmAgent      ocmagentv1alpha1.OcmAgent
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockClient = clientmocks.NewMockClient(mockCtrl)
		testOcmAgent = testconst.TestOCMAgent
		testHSOcmAgent = testconst.TestHSOCMAgent
		testOcmAgentHandler = ocmAgentHandler{
			Client: mockClient,
			Log:    testconst.Logger,
			Ctx:    testconst.Context,
			Scheme: testconst.Scheme,
		}
	})

	Context("When building an OCM Agent NetworkPolicy for MUO", func() {
		var muoNetworkPolicy netv1.NetworkPolicy

		BeforeEach(func() {
			muoNetworkPolicy = buildNetworkPolicyForMUO(testOcmAgent)
		})

		It("Should have the expected name and namespace for MUO", func() {
			Expect(muoNetworkPolicy.Name).To(Equal(testOcmAgent.Name + "-allow-muo-communication"))
			Expect(muoNetworkPolicy.Namespace).To(Equal(oah.OCMAgentNamespace))
		})

		It("Should include an ingress rule to allow traffic from the MUO namespace", func() {
			Expect(len(muoNetworkPolicy.Spec.Ingress)).To(Equal(1))
			Expect(muoNetworkPolicy.Spec.Ingress[0].From).To(HaveLen(1))

			nsSelector := muoNetworkPolicy.Spec.Ingress[0].From[0].NamespaceSelector
			Expect(nsSelector).NotTo(BeNil())
			Expect(nsSelector.MatchLabels).To(HaveKeyWithValue("kubernetes.io/metadata.name", "openshift-managed-upgrade-operator"))
		})

		It("Should apply to pods with the correct app label", func() {
			Expect(muoNetworkPolicy.Spec.PodSelector.MatchLabels).To(HaveKeyWithValue("app", testOcmAgent.Name))
		})
	})

	Context("Managing the OCM Agent NetworkPolicy for MUO", func() {
		var testMUONetworkPolicy netv1.NetworkPolicy
		var testMUONamespacedName types.NamespacedName

		BeforeEach(func() {
			testMUONetworkPolicy = buildNetworkPolicyForMUO(testOcmAgent)
			testMUONamespacedName = types.NamespacedName{
				Namespace: testMUONetworkPolicy.Namespace,
				Name:      testMUONetworkPolicy.Name,
			}
		})

		When("the MUO network policy does not already exist", func() {
			It("creates the MUO network policy", func() {
				notFound := k8serrs.NewNotFound(schema.GroupResource{}, testMUONetworkPolicy.Name)
				gomock.InOrder(
					mockClient.EXPECT().Get(gomock.Any(), testMUONamespacedName, gomock.Any()).Return(notFound),
					mockClient.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(
						func(ctx context.Context, d *netv1.NetworkPolicy, opts ...client.CreateOptions) error {
							Expect(reflect.DeepEqual(d.Spec, testMUONetworkPolicy.Spec)).To(BeTrue())
							return nil
						}),
				)
				err := testOcmAgentHandler.ensureNetworkPolicyForMUO(testOcmAgent)
				Expect(err).To(BeNil())
			})
		})

		When("the MUO network policy already exists", func() {
			When("the MUO network policy differs from what is expected", func() {
				BeforeEach(func() {
					testMUONetworkPolicy.Spec.PodSelector.MatchLabels = map[string]string{"fake": "fake"}
				})
				It("updates the MUO network policy", func() {
					gomock.InOrder(
						mockClient.EXPECT().Get(gomock.Any(), testMUONamespacedName, gomock.Any()).SetArg(2, testMUONetworkPolicy),
						mockClient.EXPECT().Update(gomock.Any(), gomock.Any()).DoAndReturn(
							func(ctx context.Context, d *netv1.NetworkPolicy, opts ...client.UpdateOptions) error {
								Expect(reflect.DeepEqual(d.Spec, buildNetworkPolicyForMUO(testOcmAgent).Spec)).To(BeTrue())
								return nil
							}),
					)
					err := testOcmAgentHandler.ensureNetworkPolicyForMUO(testOcmAgent)
					Expect(err).To(BeNil())
				})
			})
			When("the MUO network policy matches what is expected", func() {
				It("does not update the MUO network policy", func() {
					gomock.InOrder(
						mockClient.EXPECT().Get(gomock.Any(), testMUONamespacedName, gomock.Any()).SetArg(2, testMUONetworkPolicy),
					)
					err := testOcmAgentHandler.ensureNetworkPolicyForMUO(testOcmAgent)
					Expect(err).To(BeNil())
				})
			})
		})
	})

	Context("When building an OCM Agent NetworkPolicy", func() {
		var np, nph netv1.NetworkPolicy
		BeforeEach(func() {
			np = buildNetworkPolicy(testOcmAgent)
			nph = buildNetworkPolicy(testHSOcmAgent)
		})

		It("Has the expected name and namespace", func() {
			Expect(np.Name).To(Equal(testOcmAgent.Name + oah.OCMAgentNetworkPolicySuffix))
			Expect(np.Namespace).To(Equal(oah.OCMAgentNamespace))
			Expect(nph.Name).To(Equal(testHSOcmAgent.Name + oah.OCMFleetAgentNetworkPolicySuffix))
			Expect(nph.Namespace).To(Equal(oah.OCMAgentNamespace))
		})

	})

	Context("Managing the OCM Agent NetworkPolicy", func() {
		var testNetworkPolicy, testHSNetworkPolicy netv1.NetworkPolicy
		var testNamespacedName, testHSNamespacedName types.NamespacedName
		BeforeEach(func() {
			testNetworkPolicy = buildNetworkPolicy(testOcmAgent)
			testNamespacedName = types.NamespacedName{
				Namespace: testNetworkPolicy.Namespace,
				Name:      testNetworkPolicy.Name,
			}
			testHSNetworkPolicy = buildNetworkPolicy(testHSOcmAgent)
			testHSNamespacedName = types.NamespacedName{
				Namespace: testHSNetworkPolicy.Namespace,
				Name:      testHSNetworkPolicy.Name,
			}
		})
		When("the network policy already exists", func() {
			When("the network policy differs from what is expected", func() {
				BeforeEach(func() {
					testNetworkPolicy.Spec.PodSelector.MatchLabels = map[string]string{"fake": "fake"}
					testHSNetworkPolicy.Spec.PodSelector.MatchLabels = map[string]string{"fake": "fake"}
				})
				It("updates the networkpolicy", func() {
					goldenNetworkPolicy := buildNetworkPolicy(testOcmAgent)
					gomock.InOrder(
						mockClient.EXPECT().Get(gomock.Any(), testNamespacedName, gomock.Any()).SetArg(2, testNetworkPolicy),
						mockClient.EXPECT().Update(gomock.Any(), gomock.Any()).DoAndReturn(
							func(ctx context.Context, d *netv1.NetworkPolicy, opts ...client.UpdateOptions) error {
								Expect(reflect.DeepEqual(d.Spec, goldenNetworkPolicy.Spec)).To(BeTrue())
								return nil
							}),
					)
					err := testOcmAgentHandler.ensureNetworkPolicy(testOcmAgent)
					Expect(err).To(BeNil())
				})
				It("updates the fleet OA networkpolicy", func() {
					goldenNetworkPolicy := buildNetworkPolicy(testHSOcmAgent)
					gomock.InOrder(
						mockClient.EXPECT().Get(gomock.Any(), testHSNamespacedName, gomock.Any()).SetArg(2, testHSNetworkPolicy),
						mockClient.EXPECT().Update(gomock.Any(), gomock.Any()).DoAndReturn(
							func(ctx context.Context, d *netv1.NetworkPolicy, opts ...client.UpdateOptions) error {
								Expect(reflect.DeepEqual(d.Spec, goldenNetworkPolicy.Spec)).To(BeTrue())
								return nil
							}),
					)
					err := testOcmAgentHandler.ensureNetworkPolicy(testHSOcmAgent)
					Expect(err).To(BeNil())
				})
			})
			When("the networkpolicy matches what is expected", func() {
				It("does not update the networkpolicy", func() {
					gomock.InOrder(
						mockClient.EXPECT().Get(gomock.Any(), testNamespacedName, gomock.Any()).SetArg(2, testNetworkPolicy),
					)
					err := testOcmAgentHandler.ensureNetworkPolicy(testOcmAgent)
					Expect(err).To(BeNil())
				})
				It("does not update the networkpolicy for HS", func() {
					gomock.InOrder(
						mockClient.EXPECT().Get(gomock.Any(), testHSNamespacedName, gomock.Any()).SetArg(2, testHSNetworkPolicy),
					)
					err := testOcmAgentHandler.ensureNetworkPolicy(testHSOcmAgent)
					Expect(err).To(BeNil())
				})
			})
		})

		When("the OCM Agent networkpolicy does not already exist", func() {
			It("creates the networkpolicy", func() {
				notFound := k8serrs.NewNotFound(schema.GroupResource{}, testNetworkPolicy.Name)
				gomock.InOrder(
					mockClient.EXPECT().Get(gomock.Any(), testNamespacedName, gomock.Any()).Return(notFound),
					mockClient.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(
						func(ctx context.Context, d *netv1.NetworkPolicy, opts ...client.CreateOptions) error {
							Expect(reflect.DeepEqual(d.Spec, testNetworkPolicy.Spec)).To(BeTrue())
							Expect(d.ObjectMeta.OwnerReferences[0].Kind).To(Equal("OcmAgent"))
							Expect(*d.ObjectMeta.OwnerReferences[0].BlockOwnerDeletion).To(BeTrue())
							Expect(*d.ObjectMeta.OwnerReferences[0].Controller).To(BeTrue())
							return nil
						}),
				)
				err := testOcmAgentHandler.ensureNetworkPolicy(testOcmAgent)
				Expect(err).To(BeNil())
			})
			It("creates the networkpolicy for HS", func() {
				notFound := k8serrs.NewNotFound(schema.GroupResource{}, testHSNetworkPolicy.Name)
				gomock.InOrder(
					mockClient.EXPECT().Get(gomock.Any(), testHSNamespacedName, gomock.Any()).Return(notFound),
					mockClient.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(
						func(ctx context.Context, d *netv1.NetworkPolicy, opts ...client.CreateOptions) error {
							Expect(reflect.DeepEqual(d.Spec, testHSNetworkPolicy.Spec)).To(BeTrue())
							Expect(d.ObjectMeta.OwnerReferences[0].Kind).To(Equal("OcmAgent"))
							Expect(*d.ObjectMeta.OwnerReferences[0].BlockOwnerDeletion).To(BeTrue())
							Expect(*d.ObjectMeta.OwnerReferences[0].Controller).To(BeTrue())
							return nil
						}),
				)
				err := testOcmAgentHandler.ensureNetworkPolicy(testHSOcmAgent)
				Expect(err).To(BeNil())
			})
		})
	})
})
