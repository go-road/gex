-- 登录验证
SELECT * FROM `user` WHERE `user`.`username` = 'lisi' ORDER BY `user`.`id` LIMIT 1

-- 查询已匹配订单
SELECT `matched_order`.`id`,`matched_order`.`price`,`matched_order`.`qty`,`matched_order`.`amount`,`matched_order`.`match_time` FROM `matched_order` WHERE `matched_order`.`symbol_id` = 1 AND (`matched_order`.`match_time` BETWEEN 1755000360382409481 AND 1755086760382409481)