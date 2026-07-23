-- Fiscal cadastro fields the NF-e needs at the product level. WHY only these: CFOP/CST/
-- alíquota are computed at emission time by the NF-e provider, not stored on the catalog.
-- NCM is modelled per PRODUCT (99% of cases); a per-variant override is out of scope until
-- demand appears. Defaults are safe: origem 0 = mercadoria nacional, unit 'UN' = unidade.
ALTER TABLE products
    ADD COLUMN IF NOT EXISTS ncm CHAR(8),
    ADD COLUMN IF NOT EXISTS cest CHAR(7),
    ADD COLUMN IF NOT EXISTS origem SMALLINT NOT NULL DEFAULT 0 CHECK (origem BETWEEN 0 AND 8),
    ADD COLUMN IF NOT EXISTS unit TEXT NOT NULL DEFAULT 'UN';
