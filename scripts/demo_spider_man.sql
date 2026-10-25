-- Original fictional Peter Parker journal inspired by Brand New Day's isolation premise.
-- Includes imagined post-rescue contact with MJ; it does not assume restored memories.
-- These are not movie dialogue or a reconstruction of canonical scenes.
-- Premise: https://www.sonypictures.com/movies/spidermanbrandnewday
-- Seed only an empty user namespace. Reruns never restore deleted entries or
-- overwrite edits. All construction tables below are TEMP, not application schema.
PRAGMA foreign_keys = ON;
BEGIN IMMEDIATE;
CREATE TEMP TABLE spider_seed AS SELECT NOT EXISTS(SELECT 1 FROM users) AS enabled;
CREATE TEMP TABLE spider_clock AS
SELECT unixepoch('now') AS at, unixepoch('now','localtime','start of day','utc') AS midnight;
CREATE TEMP TABLE spider_days (
    day_number INTEGER, title TEXT,
    morning_subject INTEGER, morning TEXT,
    reflection_subject INTEGER, reflection TEXT,
    intention_subject INTEGER, intention TEXT
);
INSERT INTO spider_days VALUES
    (0, 'A name on an envelope', 1, 'The landlord said Parker because he was reading the rent envelope. I nearly answered like an old friend had found me. Instead I asked about the hot water. Apparently both of us were hoping somebody else would fix it.', 1, 'Bought two mugs at the thrift store. There is exactly one person in this apartment. I told myself the second one is for pencils. It is a very clean pencil mug.', 1, 'The city still knows Spider-Man. Peter Parker is currently best known to a gas bill. I should probably open that before building another gadget.'),
    (1, 'Her usual order', 3, 'MJ asked for my name for the coffee. I knew hers, her order, and the face she makes when somebody explains something she already understands. I gave her Peter. She wrote it down. I watched the pen like it was doing something impossible.', 3, 'She laughed at a joke I made about the weather. A small laugh. I spent three rooftops telling myself that was allowed to be enough, then almost hit a water tower because I was smiling.', 3, 'I miss being able to walk into a room and have her already know why I look like that. Tonight I have to explain the black eye to a mirror. The mirror is suspicious.'),
    (2, 'Something Ned would laugh at', 4, 'Saw a pigeon steal an entire slice from a man who was still holding the plate. Reached for my phone to tell Ned. There is no version of the contact list that explains why that tiny movement hurts this much.', 4, 'He used to spot the loose thread in every terrible plan and then help me do it anyway. Today I spent twenty minutes talking through a web-fluid problem with the fridge. Ned would have had better questions. The fridge had a buzzing noise.', 4, 'I do not just miss the big things. I miss sending him something stupid without needing a reason. Nobody warns you how much friendship is made of things that do not need explaining.'),
    (3, 'A very loud quiet room', 6, 'Woke up hearing the elevator three floors away. Also a couple arguing about a missing sock. Whatever is changing in me has chosen some deeply unhelpful research material.', 1, 'I can tell when something is wrong across the street, but I am still bad at noticing that I have not eaten. There is probably a very embarrassing graph in that.', 6, 'Wrote down what happened instead of deciding I could out-stubborn it. If I take the notebook to someone who understands this, I would like at least one page that is not a doodle of a sandwich.'),
    (4, 'Thank you, Spider-Man', 3, 'It was MJ. For a second the rescue stopped being distances and timing and became the horrifying fact that somebody I love was falling. I caught her. I keep writing that last part because the first part will not leave me alone.', 3, 'She said thank you, Spider-Man. I know that is a good ending to a rescue. I know. It still hurt to hear my whole life compressed into the half of me she recognized.', 5, 'Did not tell her everything while she was shaken. A secret is not made fair just because I am desperate to stop carrying it. I stayed until she was steady, then found a roof where my hands could shake.'),
    (5, 'A conversation without the mask', 3, 'Spoke to MJ as Peter. Had to remind myself that knowing her does not mean she knows me. Asked an actual question and let her answer instead of quietly comparing everything to before.', 3, 'She wanted to know how my week had been. I said busy, because the full answer involves several ambulances and a pigeon committing grand theft pizza. She said hers had been weird too. We found somewhere to start.', 3, 'There might be another dinner. That is a dinner, not a promise that the past is coming back. I am trying very hard to be a normal amount of excited about food.'),
    (6, 'Pizza, with crosswinds', 7, 'Sometimes I forget how few people get to see the city from up here. Tonight the view is mostly sideways rain. I am late bringing dinner to MJ, which apparently makes this mandatory one-handed web-slinging practice with a pizza acting as a sail. A cab would have been less spectacular. Probably warmer, too.', 3, 'I arrived with wet hair, a damp pizza box, and an explanation that was somehow less convincing than the truth. MJ moved the box away from the puddle I was making. She still has that look when she knows an explanation has missing pages.', 7, 'Dinner was good. I do not know what happens next, and I spent most of the trip home trying not to turn that into a catastrophe. I can let a good evening be a good evening. The pizza has already suffered enough.'),
    (7, 'Being late in two directions', 5, 'Promised to be somewhere at seven. Heard a crash at six fifty-eight. There is no route through that decision where I arrive feeling like a decent person to everybody.', 5, 'The person I helped got home to somebody waiting. I was glad. Then I stood outside my own door rehearsing an apology I could not explain. Those two things happened in the same ten minutes.', 5, 'I cannot keep asking people to be patient with a blank space. I also cannot give every person I love a target on their back. Tonight I have no clever conclusion. I have a cold dinner and a phone I should answer.'),
    (8, 'What nobody can see', 2, 'The reports do not add up. A threat nobody can see turns every frightened guess into a suspect. I know what it feels like to have people decide who you are before you get a word in. I need to be better than that.', 4, 'Pinned the incidents on the wall. My landlord thinks I am studying geography. At least one of us is pleased with my education.', 4, 'I miss having Ned beside the map, pointing at the thing I made complicated. Tonight I left a space beside the desk anyway. This apartment contains a remarkable number of arrangements for people who are not here.'),
    (9, 'May would have noticed', 8, 'Passed a coat drive and remembered May teaching me that help usually begins before the dramatic part. Sorted donations for an hour. Nobody fell off anything. I think she would approve of that.', 8, 'I wanted to tell her about dinner with MJ. I had the entire first sentence ready before I remembered. Grief can apparently catch up with you while you are choosing apples.', 8, 'Bought groceries instead of using the last money on an upgrade. May would call that common sense. I have found it to be a surprisingly advanced subject.'),
    (10, 'Office hours, somehow', 6, 'Took my notes to someone who might actually understand what is happening to my body. It is an uncomfortable thing, asking for help when people generally ask you to lift buses.', 6, 'Almost made a joke every time the conversation got serious. Eventually managed to finish a sentence without hiding in the punchline. Nobody awarded extra credit, which felt unfair.', 5, 'There is a difference between keeping a secret and refusing every possible hand. I am not good at finding it yet. At least the notebook now contains something besides my own handwriting.'),
    (11, 'A joke he did not know', 4, 'Ned made a joke near the coffee counter that would once have meant a whole afternoon to us. I laughed too soon. He looked over. I pretended the pastry case had excellent timing.', 4, 'Had an ordinary conversation with him. No revelation, no big moment. He explained something I helped him learn years ago, and I listened. His excitement still makes me happy. That part did not get erased.', 4, 'Being near them is not the same as getting them back. But today I heard my best friend laugh, and for a minute it was not only something I had lost.'),
    (12, 'The bit after the siren', 3, 'MJ asked whether I ever switch off. The honest answer was no, but the answer I gave her was a joke about the apartment wiring. I could hear myself doing it. I am getting tired of being good at that.', 2, 'Stayed after a rescue while a man waited for his daughter. He kept asking if she knew where he was. I said yes. I think I needed to hear it too.', 1, 'Did one fewer lap tonight. The city did not become safe, but it also did not collapse because Peter Parker went home. I am recording this surprising development for future experiments.'),
    (13, 'Room for another day', 1, 'The suit is dry, there is food in the fridge, and I have somebody I want to speak to tomorrow. None of that fixes everything. It is still a noticeably better set of problems than last week.', 3, 'Caught myself imagining an ordinary afternoon with MJ and Ned. Did not punish myself for it. I can want things without turning every hope into an assignment they have to complete.', 5, 'Tomorrow: pay the bill, check the web cartridges, ask how her day went, and stay long enough to hear the answer. Save New York somewhere in there. Very manageable list. Might need a second pen.');
INSERT INTO users(user_id,handle)
SELECT 'spider-demo','local' FROM spider_seed WHERE enabled;
INSERT INTO subjects(subject_id,user_id,subject_name)
SELECT id,'spider-demo',name FROM (
    SELECT 1 AS id,'Being Peter' AS name
UNION ALL
    SELECT 2 AS id,'The Neighborhood' AS name
UNION ALL
    SELECT 3 AS id,'MJ' AS name
UNION ALL
    SELECT 4 AS id,'Ned' AS name
UNION ALL
    SELECT 5 AS id,'Responsibility' AS name
UNION ALL
    SELECT 6 AS id,'Science & the Suit' AS name
UNION ALL
    SELECT 7 AS id,'Web Slinging' AS name
UNION ALL
    SELECT 8 AS id,'Remembering May' AS name
) CROSS JOIN spider_seed WHERE enabled;
INSERT INTO subject_aliases(subject_id,user_id,alias_name)
SELECT id,'spider-demo',name FROM (
    SELECT 1 AS id,'Home' AS name
UNION ALL
    SELECT 2 AS id,'Patrol' AS name
UNION ALL
    SELECT 3 AS id,'Michelle' AS name
UNION ALL
    SELECT 4 AS id,'My best friend' AS name
UNION ALL
    SELECT 5 AS id,'Keeping going' AS name
UNION ALL
    SELECT 6 AS id,'Science' AS name
UNION ALL
    SELECT 7 AS id,'Swinging' AS name
UNION ALL
    SELECT 8 AS id,'May' AS name
) CROSS JOIN spider_seed WHERE enabled;

CREATE TEMP TABLE spider_schedule (
    day_number INTEGER, slot INTEGER, hour INTEGER,
    subject_id INTEGER, activity TEXT, note TEXT
);
INSERT INTO spider_schedule VALUES
    (0, 0, 7, 1, 'Breakfast and the apartment', 'The smoke alarm found my toast before my spider-sense did. Good to know somebody in this apartment has a stable job.'),
    (0, 1, 9, 6, 'Science and suit repairs', 'The new web cartridge fits. The drawer where I keep the old ones does not close. Every scientific breakthrough creates furniture problems.'),
    (0, 2, 12, 1, 'Delivery shift and lunch', 'Delivered lunch to an office with a birthday cake. They asked me to join the photo, then remembered I was the delivery guy. Free cake, though.'),
    (0, 3, 16, 2, 'The neighborhood at ground level', 'Helped Mrs. Alvarez carry her groceries. She called me the skinny kid upstairs. That is almost a secret identity with references.'),
    (0, 4, 20, 7, 'Evening web-slinging patrol', 'Someone waved at the mask from a train window. I waved back and missed my next anchor. Friendly neighborhood situational awareness.'),
    (0, 5, 23, 5, 'Journal before sleep', 'The second mug is still empty. Fixed the handle anyway.'),
    (1, 0, 7, 1, 'Breakfast and the apartment', 'Made eggs without using a clean plate. Ate from the pan. My domestic efficiency research has entered a troubling phase.'),
    (1, 1, 9, 6, 'Science and suit repairs', 'Repaired the wrist trigger. It now clicks at exactly the volume of the coffee-shop door. Very helpful for concentrating on science.'),
    (1, 2, 12, 1, 'Delivery shift and lunch', 'The delivery app asked if I enjoyed my shift. It did not have a box for emotionally complicated coffee.'),
    (1, 3, 16, 2, 'Coffee where MJ works', 'Walked past her shop once. Kept walking the second time. A third pass would have required buying the building.'),
    (1, 4, 20, 7, 'Evening web-slinging patrol', 'Saved a cyclist from a taxi door. He thanked me, then asked if I could do something about bike lanes. Fair question.'),
    (1, 5, 23, 5, 'Journal before sleep', 'Practiced saying goodnight to nobody. It sounds less dramatic when the radiator answers.'),
    (2, 0, 7, 1, 'Breakfast and the apartment', 'The cereal mascot looks much too pleased with how my morning is going.'),
    (2, 1, 9, 6, 'Science and suit repairs', 'Built a mount for the scanner. Ned would call it overengineered, which is rude because it only has two backup hinges.'),
    (2, 2, 12, 1, 'Delivery shift and lunch', 'A customer tipped me in quarters. Finally, financial support for my greatest enemy: the laundromat.'),
    (2, 3, 16, 2, 'The neighborhood at ground level', 'A kid was teaching his friend a handshake. Took them five tries. Stayed nearby for the successful sixth.'),
    (2, 4, 20, 7, 'Evening web-slinging patrol', 'My scanner caught a commercial for kitchen knives. For one hopeful second, dinner sounded like an emergency I understood.'),
    (2, 5, 23, 5, 'Journal before sleep', 'Did not delete the unsent pigeon message. It is not taking up much space.'),
    (3, 0, 7, 1, 'Breakfast and the apartment', 'The kettle sounded like a fire alarm this morning. Ate breakfast on the fire escape. The kettle won the apartment.'),
    (3, 1, 9, 6, 'Science and suit repairs', 'Recorded the sensory changes, including the sock argument. Science does not get to choose glamorous data.'),
    (3, 2, 12, 1, 'Delivery shift and lunch', 'Forgot a customer''s drink because I could hear a siren six blocks away. Went back for it. Small responsibility remains responsibility.'),
    (3, 3, 16, 2, 'The neighborhood at ground level', 'Bought earplugs. The cashier said they are good for loud roommates. I said mine is the entire borough.'),
    (3, 4, 20, 7, 'Evening web-slinging patrol', 'Left patrol before the headache became an excuse to snap at somebody. Swinging home slowly feels surprisingly difficult.'),
    (3, 5, 23, 5, 'Journal before sleep', 'The notebook says ask for help. I underlined it. Apparently I am now assigning myself homework.'),
    (4, 0, 7, 1, 'Breakfast and the apartment', 'Burned the first pancake. Ate it standing up. Did not enjoy the symbolism.'),
    (4, 1, 9, 6, 'Science and suit repairs', 'Checked the web line again after the rescue. It held. I checked again anyway.'),
    (4, 2, 12, 1, 'Delivery shift and lunch', 'Sat with lunch getting cold while I replayed the landing. Somebody in the diner asked if I needed the ketchup. That helped more than expected.'),
    (4, 3, 16, 2, 'The neighborhood at ground level', 'Passed the place where it happened. The city had already put an ordinary afternoon back around it.'),
    (4, 4, 20, 7, 'A rescue that feels too close', 'Caught myself listening for her voice in every crowd. Went home before worry became an excuse to hover.'),
    (4, 5, 23, 5, 'Journal before sleep', 'My hands stopped shaking while I stitched the glove. I left the light on a little longer.'),
    (5, 0, 7, 1, 'Breakfast and the apartment', 'Cleaned the apartment in case I become the sort of person who has a guest. Hid the web-fluid notes under a rent reminder. Excellent camouflage.'),
    (5, 1, 9, 6, 'Science and suit repairs', 'Designed a quieter wrist trigger. It would be nice if my sleeve did not sound armed when I reach for a napkin.'),
    (5, 2, 12, 1, 'Delivery shift and lunch', 'A delivery customer asked why I was dressed so nicely. I was wearing a shirt without a tear. Expectations remain manageable.'),
    (5, 3, 16, 2, 'A conversation with MJ', 'Bought flowers, reconsidered, bought groceries instead. The broccoli has been given no romantic responsibilities.'),
    (5, 4, 20, 7, 'Evening web-slinging patrol', 'Kept checking the time during patrol. The city responded with a broken traffic light and a man stuck on a roof. Subtle.'),
    (5, 5, 23, 5, 'Journal before sleep', 'Wrote dinner in the calendar. Not a code name. Just dinner.'),
    (6, 0, 7, 1, 'Breakfast and the apartment', 'Checked the forecast. Rain. Checked it again, as if meteorology accepts appeals.'),
    (6, 1, 9, 6, 'Science and suit repairs', 'Waterproofed the cartridges. Neglected to waterproof myself. This is why systems need integration tests.'),
    (6, 2, 12, 1, 'Delivery shift and lunch', 'The pizza place offered delivery. Said no thanks on pure professional pride. That decision aged badly.'),
    (6, 3, 16, 2, 'The neighborhood at ground level', 'Bought a second pair of socks. Two degrees and an entire secret identity, defeated by damp feet.'),
    (6, 4, 20, 7, 'Bring dinner through the rain', 'A gust rotated the pizza ninety degrees. Managed to save it. Would appreciate the city not calling this a rescue statistic.'),
    (6, 5, 23, 5, 'Journal before sleep', 'Left the suit to dry and put the dinner receipt in a drawer. Some nights I want proof of the ordinary parts.'),
    (7, 0, 7, 1, 'Breakfast and the apartment', 'Packed a sandwich for later. Ate it immediately while looking for the bag to put it in.'),
    (7, 1, 9, 6, 'Science and suit repairs', 'A small tear in the suit became a large tear when I tested my repair. An optimistic approach to materials science.'),
    (7, 2, 12, 1, 'Delivery shift and lunch', 'Missed the good delivery window because somebody needed help carrying a stroller downstairs. Made less money. Still the right stop.'),
    (7, 3, 16, 2, 'The neighborhood at ground level', 'Put an actual buffer between two appointments. A revolutionary scheduling technology called not lying to myself.'),
    (7, 4, 20, 7, 'Evening web-slinging patrol', 'Helped clear a stalled car from an intersection. The driver said he was late too. We had a moment.'),
    (7, 5, 23, 5, 'Journal before sleep', 'Answered the phone before writing an apology three different ways. Should try that sooner next time.'),
    (8, 0, 7, 1, 'Breakfast and the apartment', 'Coffee went cold while I studied the map. Reheated it. Forgot it again. The invisible threat is clearly winning the kitchen.'),
    (8, 1, 9, 6, 'Science and suit repairs', 'Compared what the scanner actually recorded with what I remembered hearing. Memory is a dramatic collaborator.'),
    (8, 2, 12, 1, 'Delivery shift and lunch', 'Finished a delivery without checking the news halfway up the stairs. A person had ordered soup, not my ongoing investigation.'),
    (8, 3, 16, 2, 'The neighborhood at ground level', 'Asked a witness what they saw before telling them what I thought happened. Got a different answer. Needed that.'),
    (8, 4, 20, 7, 'Evening web-slinging patrol', 'Came back with three useful observations and no thrilling chase. The notebook is disappointed. I am not.'),
    (8, 5, 23, 5, 'Journal before sleep', 'Moved the map away from the radiator. The conspiracy now has fewer curling edges.'),
    (9, 0, 7, 1, 'Breakfast and the apartment', 'Made her kind of breakfast, approximately. The pan and I disagree about what happened to the eggs.'),
    (9, 1, 9, 6, 'Science and suit repairs', 'Fixed a broken desk lamp for the donation room. Not everything useful needs a wrist launcher.'),
    (9, 2, 12, 1, 'Delivery shift and lunch', 'A volunteer insisted I take a sandwich. Let somebody help me without constructing a moral argument first.'),
    (9, 3, 16, 2, 'Help at the coat drive', 'Spent twenty minutes pairing donated gloves. Mine shoot webs and I still lose one in the laundry.'),
    (9, 4, 20, 7, 'Evening web-slinging patrol', 'Stopped to check on the building after the sirens left. May always asked who was still there afterward.'),
    (9, 5, 23, 5, 'Journal before sleep', 'Wrote down one memory before I could polish it into a speech. She was funny. I want to remember that too.'),
    (10, 0, 7, 1, 'Breakfast and the apartment', 'Ate before the appointment so my stomach would not contribute to the medical history.'),
    (10, 1, 9, 6, 'Science and suit repairs', 'Organized the symptoms by time instead of how frightening they sounded. A less cinematic and considerably more useful list.'),
    (10, 2, 12, 1, 'Delivery shift and lunch', 'Worked a short shift. Kept the afternoon free without filling it with five secret backup tasks.'),
    (10, 3, 16, 2, 'Ask for help with the sensory changes', 'Arrived early enough to sit on a bench. Did not know what to do with the extra twelve minutes. Apparently sitting counts.'),
    (10, 4, 20, 7, 'Evening web-slinging patrol', 'Kept patrol short. The city survived the audacity of me following advice.'),
    (10, 5, 23, 5, 'Journal before sleep', 'Put the notes somewhere I can find them. This may be my most technically challenging repair all week.'),
    (11, 0, 7, 1, 'Breakfast and the apartment', 'The second mug finally held coffee. For me. Pencils have been displaced to a less emotionally complicated jar.'),
    (11, 1, 9, 6, 'Science and suit repairs', 'Found an old design note written as a terrible joke. Still understood it. Some documentation standards are personal.'),
    (11, 2, 12, 1, 'Delivery shift and lunch', 'Changed my route to finish near the coffee shop. Calling this logistics does not make it less obvious.'),
    (11, 3, 16, 2, 'An ordinary conversation with Ned', 'A dog recognized Spider-Man before its owner did. Tempted to ask if it remembers Peter. Decided not to put that on a dog.'),
    (11, 4, 20, 7, 'Evening web-slinging patrol', 'Tried a joke during a rescue. The person was too frightened to laugh. Stayed until they could breathe easily instead.'),
    (11, 5, 23, 5, 'Journal before sleep', 'There is no appropriate folder for a good conversation with a friend who just met you. Left it on the page.'),
    (12, 0, 7, 1, 'Breakfast and the apartment', 'Put the phone down during breakfast. Lasted four minutes. An early but promising trial.'),
    (12, 1, 9, 6, 'Science and suit repairs', 'Fixed the suit seam instead of inventing a new reason to work until dawn.'),
    (12, 2, 12, 1, 'Delivery shift and lunch', 'Delivered dinner to a family arguing affectionately over who ordered what. Missed that kind of noise on the way back down.'),
    (12, 3, 16, 2, 'The neighborhood at ground level', 'Walked home at ground level. There are small bookstores up here too, apparently, if up here means the sidewalk.'),
    (12, 4, 20, 7, 'Evening web-slinging patrol', 'Helped somebody find their brother after the crowd cleared. The quiet part of a rescue takes longer than the swinging.'),
    (12, 5, 23, 5, 'Journal before sleep', 'Left tomorrow''s breakfast on the shelf where I will actually see it. Aggressive investment in my future.'),
    (13, 0, 7, 1, 'Breakfast and the apartment', 'Bought more than one person''s worth of coffee without inventing an explanation for it. It keeps. That can be enough for now.'),
    (13, 1, 9, 6, 'Science and suit repairs', 'The waterproofing works. Tested it over the sink this time. The pizza industry thanks me for my restraint.'),
    (13, 2, 12, 1, 'Delivery shift and lunch', 'A customer remembered my name. Correctly. Tried to act like this was a normal amount of nice.'),
    (13, 3, 16, 2, 'The neighborhood at ground level', 'The kid downstairs asked if I could fix his bike. Said yes before remembering I do not own a proper wrench. Heroism requires shopping.'),
    (13, 4, 20, 7, 'Take a minute for the skyline', 'The sunset was ridiculous. Stopped on a roof and looked at it without searching for a problem inside the view.'),
    (13, 5, 23, 5, 'Journal before sleep', 'Set out both mugs, then put one away. Not a tragedy. Just making room without making a promise.');
-- Fourteen completed local calendar days plus today's open event. Use local
-- calendar arithmetic before converting to UTC, including across DST changes.
CREATE TEMP TABLE spider_dates AS
SELECT d.*,
    unixepoch(at,'unixepoch','localtime','start of day',printf('%+d days',day_number-14),'utc') AS midnight,
    unixepoch(at,'unixepoch','localtime','start of day',printf('%+d days',day_number-13),'utc') AS next_midnight
FROM spider_days d CROSS JOIN spider_clock;
CREATE TEMP TABLE spider_activities AS
SELECT s.*, d.midnight,
    unixepoch(d.midnight,'unixepoch','localtime',printf('+%d hours',hour),'utc') AS started_at
FROM spider_schedule s JOIN spider_dates d USING(day_number)
WHERE slot<>1 OR day_number%3<>0;
INSERT INTO events(event_id,user_id,subject_id,activity_type,started_at,ended_at,created_at,updated_at)
SELECT day_number*8+slot+1,'spider-demo',subject_id,activity,
    started_at,started_at+3000,started_at,started_at+3000
FROM spider_activities CROSS JOIN spider_seed WHERE enabled;

CREATE TEMP TABLE spider_notes(id INTEGER,subject_id INTEGER,body TEXT,observed INTEGER,mindset_end INTEGER);
INSERT INTO spider_notes
SELECT day_number*16+1,morning_subject,morning,
    unixepoch(midnight,'unixepoch','localtime','+7 hours','+20 minutes','utc'),next_midnight FROM spider_dates
UNION ALL
SELECT day_number*16+2,reflection_subject,reflection,
    unixepoch(midnight,'unixepoch','localtime','+12 hours','+20 minutes','utc'),next_midnight FROM spider_dates
UNION ALL
SELECT day_number*16+3,intention_subject,intention,
    unixepoch(midnight,'unixepoch','localtime','+23 hours','+20 minutes','utc'),next_midnight FROM spider_dates;
INSERT INTO spider_notes
SELECT day_number*16+slot+4,subject_id,note,started_at+1800,NULL FROM spider_activities;
INSERT INTO events(event_id,user_id,subject_id,activity_type,started_at,created_at,updated_at)
SELECT 200,'spider-demo',1,'A little room for Peter',
    max(midnight,at-2700),max(midnight,at-2700),max(midnight,at-2700)
FROM spider_clock CROSS JOIN spider_seed WHERE enabled;
INSERT INTO spider_notes
SELECT 225,1,'The pizza box is still by the door. Apparently I kept a souvenir of carrying dinner badly through Manhattan. There are worse reasons to clean the apartment.',max(midnight,at-2400),NULL FROM spider_clock
UNION ALL
SELECT 226,4,'Saw something Ned would laugh at. Wrote it here instead of pretending that remembering him is the problem.',max(midnight,at-1500),NULL FROM spider_clock
UNION ALL
SELECT 227,7,'Clear patch over the river. Before the next siren, I want one minute of this view without turning it into a route to somewhere else.',max(midnight,at-600),NULL FROM spider_clock;
INSERT INTO thoughts(thought_id,user_id,subject_id,thought,observed_at,created_at,updated_at)
SELECT id,'spider-demo',subject_id,body,observed,observed,observed
FROM spider_notes CROSS JOIN spider_seed WHERE enabled;
-- The first three notes of each day are its initial Mindsets. Historical periods
-- close at the following local midnight; today's three remain active.
INSERT INTO thought_mindset_periods(thought_id,started_at,ended_at)
SELECT id,observed,mindset_end FROM spider_notes CROSS JOIN spider_seed
WHERE enabled AND (id%16 BETWEEN 1 AND 3);
DROP TABLE spider_notes;
DROP TABLE spider_activities;
DROP TABLE spider_dates;
DROP TABLE spider_schedule;
DROP TABLE spider_days;
DROP TABLE spider_clock;
DROP TABLE spider_seed;
COMMIT;
