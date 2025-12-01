terraform init
terraform plan
terraform apply

terraform output -raw private_key > secrets/private_key.pem
terraform output -raw certificate_pem > secrets/device_certificate.crt