resource "aws_dynamodb_table" "smart_home_state" {
  name         = "smart_home_state"
  billing_mode = "PAY_PER_REQUEST"
  table_class  = "STANDARD"

  hash_key  = "state_id"
  range_key = "timestamps"

  attribute {
    name = "state_id"
    type = "N"
  }

  attribute {
    name = "timestamps"
    type = "N"
  }

  # No backups
  point_in_time_recovery {
    enabled = false
  }

  server_side_encryption {
    enabled = true
  }

  tags = {
    Environment = "dev"
    Project     = "smart-home"
  }
}
