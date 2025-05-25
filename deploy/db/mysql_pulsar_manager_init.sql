CREATE DATABASE `pulsar_manager` /*!40100 DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci */ /*!80016 DEFAULT ENCRYPTION='N' */;

-- DROP USER 'pulsar'@'%';
CREATE USER 'pulsar'@'%' IDENTIFIED BY 'pulsar';

GRANT ALL ON pulsar_manager.* TO 'pulsar'@'%';

USE pulsar_manager;
-- 用户名：admin
-- 密码：pulsar
-- ----------------------------
-- Records of role_binding
-- ----------------------------
INSERT INTO `role_binding` VALUES (1, 'super_user_role_binding', 'This is super role binding', 2, 1);

-- ----------------------------
-- Records of roles
-- ----------------------------
INSERT INTO `roles` VALUES (2, 'admin', 'admin', 'This is super role', 0, 'ALL', 'superuser', 'SUPER_USER', 0);

-- ----------------------------
-- Records of users
-- ----------------------------
INSERT INTO `users` VALUES (1, 'eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJhZG1pbmUwMDgxMWJiNTM2MzBlM2JiNjVhNTUzODBlN2JiNmM0MjdiOGVmODk1MDE1OTE1ZDc3MTFhOWZiOWM5MzY2MGExNzQ4MDg3NDY4NDU5IiwiZXhwIjoxNzQ2Mzg0NTAxfQ.mo9Sw3hYVvDTGo8LD0-g9Fw-zpLeTItyaI33KbTL25g', 'admin', 'test', 'username@test.org', NULL, NULL, NULL, '0', 'e00811bb53630e3bb65a55380e7bb6c427b8ef895015915d7711a9fb9c93660a');
