-- Rollback 000026: drop internal LB tables

DROP TABLE IF EXISTS lb_backend_health;
DROP TABLE IF EXISTS load_balancers;
