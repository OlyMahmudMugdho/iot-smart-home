output "certificate_pem" {
  value     = aws_iot_certificate.device.certificate_pem
  sensitive = true
}

output "private_key" {
  value     = aws_iot_certificate.device.private_key
  sensitive = true
}

output "public_key" {
  value     = aws_iot_certificate.device.public_key
  sensitive = true
}

output "iot_endpoint_address" {
  value = data.aws_iot_endpoint.iot_data_ats_endpoint.endpoint_address
  description = "The address of the AWS IoT Data-ATS endpoint."
}