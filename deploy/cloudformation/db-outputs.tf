output "this_db_instance_address" {
  description = "The address of the RDS instance"
  value       = "${module.db.this_db_instance_address}"
}

output "this_db_instance_endpoint" {
  description = "The connection endpoint"
  value       = "${module.db.this_db_instance_endpoint}"
}

output "this_db_instance_hosted_zone_id" {
  description = "The canonical hosted zone ID of the DB instance (to be used in a Route 53 Alias record)"
  value       = "${module.db.this_db_instance_hosted_zone_id}"
}

output "this_db_instance_id" {
  description = "The RDS instance ID"
  value       = "${module.db.this_db_instance_id}"
}

output "this_db_instance_resource_id" {
  description = "The RDS Resource ID of this instance"
  value       = "${module.db.this_db_instance_resource_id}"
}

output "this_db_instance_username" {
  description = "The master username for the database"
  value       = "${module.db.this_db_instance_username}"
}

output "this_db_instance_port" {
  description = "The database port"
  value       = "${module.db.this_db_instance_port}"
}

