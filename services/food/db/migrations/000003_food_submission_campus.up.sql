-- Food Submission campus assignment (#413).
--
-- Console ops may assign which campus a pending submission belongs to when
-- editing its content; NULL means not yet assigned. The enum matches the
-- Food Post campus values (minglun/jinming/longzihu).

ALTER TABLE food_submissions
    ADD COLUMN IF NOT EXISTS campus text CHECK (campus IN ('minglun','jinming','longzihu'));
