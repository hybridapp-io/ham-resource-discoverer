FROM registry.access.redhat.com/ubi8/ubi-minimal:latest

# Update image
RUN microdnf update -y && microdnf clean all

LABEL   name="quay.io/cicdtest/ham-resource-discoverer" \
        summary="Hybrid Application Model - Resource Discoverer Operator" \
        description="Works with Virtualization Operators (KubeVirt, CloudForms, etc) in a managed cluster for deployments and day2 actions."

ENV OPERATOR=/usr/local/bin/ham-resource-discoverer \
    USER_UID=1001 \
    USER_NAME=ham-resource-discoverer

# install operator binary
COPY build/_output/bin/ham-resource-discoverer ${OPERATOR}

COPY build/bin /usr/local/bin
RUN  /usr/local/bin/user_setup

# copy licenses
RUN mkdir /licenses
COPY LICENSE /licenses

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}
