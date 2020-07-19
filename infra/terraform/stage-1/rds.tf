resource aws_db_instance tripbot {
  engine         = "postgres"
  engine_version = "11.6"
  instance_class = "db.t2.micro"

  identifier = "tripbot-db"
  name       = "tripbot"
  username   = var.rds_tripbot_username
  password   = var.rds_tripbot_password

  allocated_storage = 20
  storage_type      = "gp2" # general purpose SSD
  # storage_encrypted = true

  # enabled_cloudwatch_logs_exports = ["postgresql", "upgrade"]

  publicly_accessible = true
  #db_subnet_group_name   = module.vpc.database_subnet_group
  #vpc_security_group_ids = [aws_security_group.allow_postgres_access.id]

  backup_retention_period   = 30
  final_snapshot_identifier = "tripbot-db-final-snapshot"
}
