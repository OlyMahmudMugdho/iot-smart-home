resource "aws_iot_thing" "device" {
  name = "SmartHomeESP32"
}

resource "aws_iot_certificate" "device" {
  active = true
}

resource "aws_iot_policy" "device_policy" {
  name = "SmartHomeESP32Policy"
  policy = <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "iot:Connect",
        "iot:Publish",
        "iot:Subscribe",
        "iot:Receive"
      ],
      "Resource": "*"
    }
  ]
}
EOF
}

resource "aws_iot_policy_attachment" "attach_policy" {
  policy = aws_iot_policy.device_policy.name
  target = aws_iot_certificate.device.arn
}

resource "aws_iot_thing_principal_attachment" "attach_cert" {
  thing     = aws_iot_thing.device.name
  principal = aws_iot_certificate.device.arn
}


data "aws_iot_endpoint" "iot_data_ats_endpoint" {
  endpoint_type = "iot:Data-ATS"
}