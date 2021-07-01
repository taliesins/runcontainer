ARG PYTHON_VERSION=3.9.6
FROM python:${PYTHON_VERSION}-slim-buster as build

WORKDIR /tmp

RUN apt-get update \
    && apt-get install -y \
    ca-certificates \
    curl \
    git \
    unzip \
    jq \
    && apt-get clean  \
    && rm -rf /var/lib/apt

    # gawk \
    # software-properties-common \
    # build-essential \
    # libxml2-dev \
    # libxslt1-dev \
    # nodejs \
    # xz-utils \
    # zlib1g-dev && \

# Get AWS CLI
ARG ANSIBLE_VERSION=4.2.0
RUN pip install --no-cache-dir ansible==${ANSIBLE_VERSION}

# Get Azure CLI
ARG AZURECLI_VERSION=2.25.0
RUN pip install --no-cache-dir azure-cli==${AZURECLI_VERSION}

# Get AWS CLI
ARG AWSCLI_VERSION=1.19.103
RUN pip install --no-cache-dir awscli==${AWSCLI_VERSION}

# Get pre-commit
ARG PRECOMMIT_VERSION=2.13.0
RUN pip install --no-cache-dir pre-commit==${PRECOMMIT_VERSION}

# Get kubectl
ARG KUBECTL_VERSION=1.21.0
RUN curl -sS -L https://storage.googleapis.com/kubernetes-release/release/v${KUBECTL_VERSION}/bin/linux/amd64/kubectl -o /usr/bin/kubectl \
	&& chmod +x /usr/bin/kubectl

# Get helm
ARG HELM_VERSION=3.6.2
RUN curl -sS -L https://get.helm.sh/helm-v${HELM_VERSION}-linux-amd64.tar.gz -o helm.tgz \
    && tar xzf helm.tgz \
    && mv linux-amd64/helm /usr/bin/ \
    && chmod +x /usr/bin/helm \
    && rm * -rf

# Get oc
ARG OC_VERSION=4.7
RUN curl -sS -L https://mirror.openshift.com/pub/openshift-v4/clients/ocp/stable-${OC_VERSION}/openshift-client-linux.tar.gz -o openshift-client-linux.tar.gz \
    && tar xzf openshift-client-linux.tar.gz \
    && mv oc /usr/bin/ \
    && chmod +x /usr/bin/oc \
    && rm * -rf

# Get rosa
ARG ROSA_VERSION=1.0.5
RUN curl -sS -L https://mirror.openshift.com/pub/openshift-v4/amd64/clients/rosa/${ROSA_VERSION}/rosa-linux.tar.gz -o openshift-client-linux.tar.gz \
    && tar xzf openshift-client-linux.tar.gz \
    && mv rosa /usr/bin/ \
    && chmod +x /usr/bin/rosa \
    && rm * -rf

# Get terraform
ARG TF_VERSION=1.0.1
RUN curl -sS -L https://releases.hashicorp.com/terraform/${TF_VERSION}/terraform_${TF_VERSION}_linux_amd64.zip -o terraform.zip \
	&& unzip terraform.zip \
	&& mv terraform /usr/bin/terraform \
    && chmod +x /usr/bin/terraform \
    && rm * -rf
	
# Get terragrunt
ARG TG_VERSION=0.31.0
RUN curl -sS -L https://github.com/gruntwork-io/terragrunt/releases/download/v${TG_VERSION}/terragrunt_linux_amd64 -o /usr/bin/terragrunt \
	&& chmod +x /usr/bin/terragrunt

# Get terraform-docs
ARG TERRAFORMDOCS_VERSION=0.14.1
RUN curl -sS -L https://github.com/terraform-docs/terraform-docs/releases/download/v${TERRAFORMDOCS_VERSION}/terraform-docs-v${TERRAFORMDOCS_VERSION}-linux-amd64.tar.gz -o terraform-docs.tgz \
    && tar xzf terraform-docs.tgz \
    && mv terraform-docs /usr/bin/ \
    && chmod +x /usr/bin/terraform-docs \
    && rm * -rf

# Get tflint
ARG TFLINT_VERSION=0.29.1
RUN curl -sS -L https://github.com/terraform-linters/tflint/releases/download/v${TFLINT_VERSION}/tflint_linux_amd64.zip -o tflint.zip \
    && unzip tflint.zip \
    && rm tflint.zip \
    && mv tflint /usr/bin/ \
    && chmod +x /usr/bin/tflint \
    && rm * -rf

# Get tfsec
ARG TFSEC_VERSION=0.40.6
RUN curl -sS -L https://github.com/tfsec/tfsec/releases/releases/download/v${TFSEC_VERSION}/tfsec-linux-amd64 -o /usr/bin/tfsec \
	&& chmod +x /usr/bin/tfsec

# Get terrascan
ARG TERRASCAN_VERSION=1.7.0
RUN curl -sS -L https://github.com/accurics/terrascan/releases/download/v${TERRASCAN_VERSION}/terrascan_${TERRASCAN_VERSION}_Linux_x86_64.tar.gz -o terrascan.tar.gz \
    && tar -xf terrascan.tar.gz terrascan \
    && rm terrascan.tar.gz \
    && mv terrascan /usr/bin/ \
    && chmod +x /usr/bin/terrascan \
    && rm * -rf

# Get checkov
ARG CHECKOV_VERSION=2.0.242
RUN pip install --no-cache-dir checkov==${CHECKOV_VERSION}

WORKDIR /config

CMD ["terragrunt", "--version"]