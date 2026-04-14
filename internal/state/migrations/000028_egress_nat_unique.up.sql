-- 1ネットワークに nat_gateway は1つまで（複数あると SNAT フローが不定になる）
CREATE UNIQUE INDEX egresses_network_nat_gateway_unique
    ON egresses (network_id)
    WHERE type = 'nat_gateway';
