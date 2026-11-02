-- Reservations survive unlinking; subject_goals alone determines membership.
CREATE TABLE subject_goal_numbers (
 subject_id INTEGER NOT NULL,
 goal_id INTEGER NOT NULL,
 user_id TEXT NOT NULL,
 goal_number INTEGER NOT NULL CHECK(typeof(goal_number) = 'integer' AND goal_number > 0),
 PRIMARY KEY(subject_id, goal_id),
 UNIQUE(subject_id, goal_number),
 FOREIGN KEY(subject_id,user_id) REFERENCES subjects(subject_id,user_id) ON DELETE CASCADE ON UPDATE CASCADE,
 FOREIGN KEY(goal_id,user_id) REFERENCES goals(goal_id,user_id) ON DELETE CASCADE ON UPDATE CASCADE
);
INSERT INTO subject_goal_numbers(subject_id,goal_id,user_id,goal_number)
 SELECT subject_id,goal_id,user_id,row_number() OVER(PARTITION BY subject_id ORDER BY goal_id)
 FROM subject_goals;
