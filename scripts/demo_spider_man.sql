-- Original comic diary around Brand New Day's plot; fictional fourteen-day spacing.
-- Story order: forgotten identity, changing powers, Banner and the inhibitors,
-- MJ's rescue, Hulk, dinner, Sara/Metzger, accepting the mutations, recovery.
-- Dialogue and day-to-day mishaps are original, not quotations or canonical scenes.
-- Plot/powers references:
-- https://www.sonypictures.com/movies/spidermanbrandnewday
-- https://thedirect.com/article/spider-man-brand-new-day-promo-three-new-power-upgrades
-- https://www.thewrap.com/creative-content/movies/spider-man-brand-new-day-inhibitor-explained/
-- https://en.wikipedia.org/wiki/Spider-Man:_Brand_New_Day
-- Seed only an empty user namespace. Existing entries and edits stay untouched.
-- All construction tables are TEMP, not application schema.
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
    (0, 'Everybody forgot Peter. Nobody forgot the deposit.', 1, 'Doctor Strange erased me from everyone''s memory. My landlord still wants first month, last month, and a security deposit. Apparently the one thing stronger than sorcery is a New York lease.', 1, 'The delivery app asked for an emergency contact. I put my own number. It rejected that. Very thorough app. Terrible bedside manner.', 5, 'Spent all night being a friendly neighborhood Spider-Man. Got home and realized I hadn''t said a sentence as Peter since buying toilet paper. That sentence was ''No, the cheaper one.'''),
    (1, 'Frank Castle has several terrible suggestions', 5, 'Frank''s idea of a distraction and my idea of a distraction require very different insurance policies.', 5, 'There is a tank involved. My plan was to avoid a tank being involved. We are experiencing a small but significant departure from the plan.', 6, 'The driver wasn''t acting like himself. Then somebody else wasn''t acting like themselves. I really miss bad guys who stay inside the bad guy I am already chasing.'),
    (2, 'Ned made an app to find the guy he used to text', 4, 'Ned built a Spidey Tracker. My best friend has reinvented asking where I am, except now it needs location permissions and drains the battery.', 3, 'MJ and Ned are back from MIT, looking at apartments. MJ has someone else. I know nobody did anything wrong. My stomach would like to open a formal dispute anyway.', 4, 'I used to be in their future. Now I know where Ned would put the desk and have no normal reason to know it. Said ''nice light.'' I have become a real-estate brochure with grief.'),
    (3, 'My wrists have stopped respecting the equipment list', 6, 'Took the web shooters off. Fired a web anyway. Checked the shooters again in case they had become wireless. They had not. This is coming out of ME. Would have preferred a memo.', 6, 'Can hear conversations through walls now. Useful for danger. Less useful for discovering exactly how many strangers are discussing my landing technique.', 5, 'Lost control fighting Gargan. A cop got hurt. I don''t get to put that under weird new powers and move on. Need help before somebody else pays for whatever is happening inside me.'),
    (4, 'Office hours with the other guy', 6, 'Need to ask Banner about a biological transformation without saying ''spider puberty.'' Have now rehearsed the words spider puberty twelve times. This is going extremely badly.', 6, 'Banner asked how I''d decide which changes were bad. I had a very convincing answer prepared until he asked that actual question.', 6, 'He knows what it''s like to worry about becoming something you can''t control. Almost hid behind a joke instead of thanking him. Managed the thanks. Needed to say it without the mask.'),
    (5, 'Back to the old powers. Still the old problems.', 6, 'Working on the inhibitor. Would like exactly one part of my biology to have an off switch without the rest of me becoming a cautionary diagram.', 6, 'The inhibitor brings the new changes under control. Back to the powers I recognize. Including the ability to run out of web fluid immediately after spending all my money on an inhibitor.', 3, 'An unsent letter to MJ was supposed to stay in my apartment. Now someone who can move through people knows about her. I wanted to tell her myself. That feels like a very small problem compared with getting her safe.'),
    (6, 'Thank you, Spider-Man. Please don''t hear my heart.', 3, 'Need to get MJ clear of this without making her feel like the last person to be told what is happening to her. Not sure how to do that. Very sure I should already be moving.', 6, 'Jean Grey. A person, a name, somebody behind all the jumping between bodies. Knowing that should make this easier. It has not made the people she can reach any less real.', 3, 'MJ said thank you to Spider-Man. I caught her. She''s safe for now. Still had to find somewhere to put the face she couldn''t see.'),
    (7, 'Professor Banner''s office hours have become office blocks', 6, 'Making the universal inhibitor. A device that could stop somebody''s powers is a much larger sentence than a device that could fix my problem. I liked the smaller sentence.', 5, 'The plan depends on reaching people before someone else gets inside their heads. Would love to return to a villain whose entire strategy is a tail.', 6, 'Banner wasn''t himself. Got people clear before anything else. Knowing who somebody is supposed to be doesn''t make the car flying toward you any less real.'),
    (8, 'Dinner with two people who don''t remember the third', 3, 'MJ found the letter. There are embarrassment scenarios I rehearsed, but none covered the actual letter getting a speaking part.', 3, 'Told her I love her. She didn''t suddenly remember. Didn''t owe me an answer from the version of her I keep carrying around. I know that. I just wanted to stop hiding the sentence.', 4, 'She invited me over. Ned too. Not a reunion where everything came back. A real evening with the people who are here. Also a dangerously large quantity of pizza.'),
    (9, 'The one time comparing terrible experiences is useful', 3, 'MJ doesn''t remember what happened while she was possessed. That doesn''t fit Metzger''s story about Sara. Trying to be suspicious for an actual reason this time instead of because he dresses like somebody who calls people assets.', 5, 'Yelena has better ways of getting answers than asking politely and being ignored. My contribution appears to be damp footwear and an increasingly specific list of questions.', 5, 'Sara is Jean''s sister. This isn''t just a mystery with a convenient bad guy at the end. Somebody is missing. Need to find her, not just win the argument about who''s lying.'),
    (10, 'Taking the inhibitor off', 6, 'Roosevelt Island. Need a plan that doesn''t boil down to get there fast and hope my body knows the rest. That plan has some mixed historical results.', 5, 'The field caught everybody except me. All these people, still. MJ and Ned among them. I can feel what''s happening, but I can''t just carry a whole frozen city out of the way.', 6, 'Took the inhibitor off. I built that thing because I was scared of becoming something I couldn''t control. Still scared. But the part I''d been trying to silence was the part I needed to reach them.'),
    (11, 'A memory, then a very bad noise', 5, 'Jean is furious and hurt. There''s a version of me that understands wanting somebody to pay for the person you can''t get back. Doesn''t mean I can let her do it.', 8, 'Tried to reach her with the part of me I''ve spent four years keeping out of sight. Not a threat, not another machine. May. How it felt to be loved without earning it by saving somebody first.', 5, 'Saw what Frank was about to do and moved. There wasn''t time to debate it. Apparently the body that can manufacture its own webs has retained an extremely inconvenient opinion about bullets.'),
    (12, 'Superhuman healing does not include hospital food', 1, 'Woke up in a hospital. Frank was there. Thought I was having a particularly judgmental afterlife until he started talking.', 5, 'Jean kept me alive. The person I was told to stop, the person I tried to understand, the person I stepped in front of. Not a very tidy category system. Glad I didn''t trust the tidy one.', 1, 'The doctor asked if I had someone to take me home. This time the question didn''t quite have the same answer it had on that delivery form.'),
    (13, 'Recovery is a terrible excuse for not being everywhere', 8, 'Going to see May. There are things I want to tell her that don''t fit in a good joke. Will probably tell the bad jokes too. She always tolerated the early drafts.', 5, 'A robbery came over the radio. DeWolff can handle a robbery. I know she can. My entire nervous system has asked to present a forty-slide rebuttal.', 4, 'Met Ned as Peter. The handshake happened. He didn''t suddenly remember everything. But his hand knew what to do, and for a second I forgot to be careful about how happy I looked.');
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
    SELECT 6 AS id,'New Powers' AS name
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
    SELECT 6 AS id,'Organic Webbing' AS name
UNION ALL
    SELECT 6 AS id,'Spider Sense' AS name
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
    (0, 0, 7, 1, 'Ceiling yoga and the smoke alarm', 'Stuck to the ceiling turning off the smoke alarm. Told the landlord I was doing yoga. He said I was doing it during quiet hours.'),
    (0, 1, 9, 6, 'Price web fluid without a Stark card', 'Priced the chemicals for another batch of web fluid. Saving a bus costs more than taking one. Several of my financial problems are becoming clear.'),
    (0, 2, 12, 1, 'Lunch deliveries from the wrong direction', 'One-star delivery review: ''Arrived from above. Unsettling.'' The dumplings were upright. I would like that entered into the record.'),
    (0, 3, 16, 2, 'Carry an air conditioner, fumble small talk', 'Helped a woman carry an air conditioner. She offered to introduce her granddaughter. Asked if the granddaughter had rent-controlled housing. Made it weird.'),
    (0, 4, 20, 7, 'Spider-Man considers Taskrabbit', 'Rescued a guy from a fire escape. He asked if Spider-Man does Taskrabbit. Technically yes, but nobody has located the task payment.'),
    (0, 5, 23, 5, 'Unstick myself from a receipt', 'Fell asleep wearing one web shooter. Woke up attached to a receipt for noodles. Somehow every part of that is embarrassing.'),
    (1, 0, 7, 1, 'Breakfast before the tank problem', 'Breakfast standing up. Saves time until the chair I didn''t use becomes the thing I hit while leaving in a hurry.'),
    (1, 1, 9, 6, 'Build Frank-proof web restraints', 'Reinforced the web restraints. Would like the nonlethal option to survive Frank''s peer review, which appears to involve a knife.'),
    (1, 2, 12, 1, 'Contactless lunch, preferably bulletless', 'Customer requested contactless delivery. Finally, a normal person who appreciates the benefits of staying outside a line of fire.'),
    (1, 3, 16, 5, 'Explain nonlethal to Frank Castle', 'Frank called me kid while explaining a felony. I explained leverage. Neither of us felt adequately respected.'),
    (1, 4, 20, 5, 'Stop a tank and chase a moving mind', 'Stopped Frank and chased the possessed driver. Apparently being the responsible adult now includes telling a much older adult to stop shooting things.'),
    (1, 5, 23, 6, 'Figure out why the telepath can''t get in', 'Whoever is jumping between people couldn''t do it to me. First time being difficult to get close to has come with a tactical advantage.'),
    (2, 0, 7, 1, 'Bagel, with a Spidey Tracker alert', 'Bought a bagel. His app buzzed that Spider-Man was near the deli. Congratulations to Ned on building technology that can tell me I am buying a bagel.'),
    (2, 1, 9, 6, 'Anonymously debug Ned''s tracker', 'The tracker drew a route through two buildings. Sent an anonymous bug report. Could not resist helping him debug my own manhunt.'),
    (2, 2, 12, 1, 'Avoid sightings by using actual stairs', 'Took the stairs to avoid generating a sighting. Six floors later I have renewed respect for absolutely everyone else.'),
    (2, 3, 16, 4, 'Ned demonstrates tracking his best friend', 'He showed me the app. I said ''Wow, real-time updates.'' He said ''Mostly.'' Both being very polite about one of us being the server.'),
    (2, 4, 20, 3, 'Patrol after seeing MJ''s new life', 'Saw them together and missed the next anchor. The mask hides a lot. It doesn''t improve spatial awareness while having a terrible evening.'),
    (2, 5, 23, 4, 'Read a notification from Ned', 'One notification said Spider-Man had left the area. Yeah, man. Usually that''s the part where I wish I had somewhere to go.'),
    (3, 0, 7, 1, 'Escape my own web-covered bedding', 'Woke up tangled in webbing with nobody else in the room to blame. Disappointing start for my crime-scene investigation.'),
    (3, 1, 9, 6, 'Test organic webs without the shooters', 'E.V. confirmed organic webs. My body has opened a surprise manufacturing department and outsourced the explanation to the computer. No cartridge bill, though. First terrifying medical development to improve my budget.'),
    (3, 2, 12, 1, 'Attempt a conversation with regular ears', 'Customer said ''regular delivery entrance.'' Replied ''regular ears.'' Had heard someone calling from upstairs. Need a lawyer for my mouth.'),
    (3, 3, 16, 6, 'Check my changing eyes in the mirror', 'My eyes looked wrong in the mirror. Tried better lighting. Excellent, now I can be frightened in high definition.'),
    (3, 4, 20, 6, 'Lose control fighting Gargan', 'Moved faster than I expected and hit harder than I meant to. Winning is not the word for how that felt.'),
    (3, 5, 23, 6, 'Record the symptoms and the injured officer', 'Wrote down the symptoms and what happened to the officer. No funny labels in that part of the notebook.'),
    (4, 0, 7, 1, 'Breakfast before Banner''s office hours', 'Ate before the appointment so my stomach wouldn''t contribute to the medical history.'),
    (4, 1, 9, 6, 'Bring power-change notes to Banner', 'Brought samples and notes. Didn''t bring the web-covered bedsheet. There are boundaries to scientific collaboration, even if I am still learning them.'),
    (4, 2, 12, 1, 'Complete an awkward university visitor form', 'University form asked for an affiliation. Wrote independent researcher. Did not write radioactive bite recipient. Networking is about presentation.'),
    (4, 3, 16, 6, 'Learn that Banner has a stress poster', 'There''s a stress-management poster near Banner''s office. Whoever put it there either knows nothing or is the funniest person on campus.'),
    (4, 4, 20, 6, 'Measure the new agility, overshoot', 'New agility is great until the distance I meant to jump and the distance my legs chose become two separate addresses.'),
    (4, 5, 23, 6, 'Price an inhibitor I can''t afford', 'Made a parts list for an inhibitor. The gadget may fit on my body. Its price tag does not fit anywhere in my life.'),
    (5, 0, 7, 1, 'Solder an inhibitor, possibly my breakfast', 'Found solder on my breakfast plate. Great. The cereal now has trace amounts of a science fair.'),
    (5, 1, 9, 6, 'Test suppressing the new mutations', 'Tested the inhibitor. The ordinary web shooters suddenly look less like obsolete equipment and more like coworkers I shouldn''t have insulted.'),
    (5, 2, 12, 1, 'Cancel deliveries: MJ may be in danger', 'Couldn''t finish the shift. App asked for a reason. Its dropdown does not cover telepath discovered my deeply embarrassing unsent love letter.'),
    (5, 3, 16, 3, 'Try calling MJ without sounding terrified', 'Tried calling MJ in a voice that didn''t sound like an emergency. My voice has declined to participate in that plan.'),
    (5, 4, 20, 6, 'Pursue the body-hopping threat', 'You can dodge a punch. You can''t just punch the person whose body somebody else is using. This job needs a completely different training course.'),
    (5, 5, 23, 3, 'Realize a letter exposed MJ', 'Keep thinking about that letter lying out in the open. Four years hiding myself, and the most dangerous thing in the apartment was paper.'),
    (6, 0, 7, 1, 'Drink both coffees, regret the senses', 'Bought two coffees by habit. Drank both. Now I can hear colors, which is an unwelcome feature given the week I''m having.'),
    (6, 1, 9, 6, 'Retest the line that has to hold MJ', 'Checked the line again. It could hold a car. Asked if it was sure. Materials science has become a needy relationship.'),
    (6, 2, 12, 1, 'Eat something possibly intended as lunch', 'Forgot what lunch was. Ate something from the backpack. Hoping it was the item I''d put there as lunch.'),
    (6, 3, 16, 3, 'Rescue MJ from Jean''s control', 'Caught MJ. Not her attention. Her. Falling. My brain is still trying to sort that into an event that finished.'),
    (6, 4, 20, 3, 'Get MJ to Frank''s hideout', 'Frank has a hideout. Of course Frank has a hideout. Mine has a radiator that bangs, so I am trying not to be judgmental about his decorating.'),
    (6, 5, 23, 6, 'Work on the inhibitors with MJ', 'MJ helped with the inhibitor work. I missed talking through a problem with her so much I almost forgot how bad the problem was.'),
    (7, 0, 7, 1, 'Pack for a day involving the Hulk', 'Packed a fresh cartridge and a spare shirt. One of those is wildly optimistic equipment for a day involving the Hulk.'),
    (7, 1, 9, 6, 'Review the universal inhibitor''s risks', 'Looked at the universal inhibitor and tried to imagine it in somebody else''s hands. Not the most comforting quality-assurance procedure.'),
    (7, 2, 12, 1, 'Lose an argument with my fitness watch', 'Phone says eight hundred steps. Carried a person three blocks and swung half of Manhattan. My watch recommends stairs.'),
    (7, 3, 16, 5, 'Clear people from the Hulk''s path', 'Helped evacuate a shop. Owner insisted on a receipt for the water. Even a superhuman disaster has paperwork.'),
    (7, 4, 20, 5, 'Fight Hulk with insufficient cover', 'Ducked behind a car. Remembered this is the Hulk, so a car is mostly a very expensive way to die behind a car. Moved.'),
    (7, 5, 23, 6, 'Remember Jean saying Sara', 'Jean said Sara. Metzger had an explanation ready. At the time I was mostly trying to make sure everybody was still breathing. Need to remember to think about it.'),
    (8, 0, 7, 1, 'Rehearse saying hello after the letter', 'Practiced a normal hello in the mirror. Spider-sense stayed silent. Apparently it only covers threats to my physical wellbeing.'),
    (8, 1, 9, 6, 'Plan dinner in rain and crosswinds', 'Forecast says rain. My wrists say I own two perfectly good methods of transport. My dry clothes would like a vote.'),
    (8, 2, 12, 1, 'Refuse pizza delivery on professional pride', 'The pizza place offered delivery. Said I''d handle it. The words were out before my brain remembered delivery exists as a profession for a reason.'),
    (8, 3, 16, 3, 'Buy pizza with the cartridge budget', 'Spent the cost of a replacement cartridge on good pizza. Physically painful calculation. Still bought the good one.'),
    (8, 4, 20, 7, 'One-arm web-slinging with dinner for MJ and Ned', 'Sometimes I forget how few people get this view of New York. Currently the view is rain inside my mask while I do mandatory one-arm web-slinging practice with dinner for MJ and Ned in the other hand. The pizza is trying to become a kite. A cab. I could have been a person in a cab.'),
    (8, 5, 23, 3, 'Dinner with two friends who don''t remember me', 'MJ laughed at my wet hair. Ned had questions about swinging. For a while I got to be in the room instead of missing it. Pizza was lukewarm. Nobody made it the important thing.'),
    (9, 0, 7, 1, 'Breakfast and an alarming number of questions', 'Breakfast plan: cereal, rent reminder, existential dread. Finally using all three of my bowls.'),
    (9, 1, 9, 6, 'Consider who else could use the inhibitor', 'The inhibitor works. That is starting to worry me almost as much as the week when it didn''t exist.'),
    (9, 2, 12, 1, 'Think about a sandwich instead of eating it', 'Stopped for a sandwich and realized I''d been holding it while thinking about the case. Good grip, terrible eating technique. On brand.'),
    (9, 3, 16, 5, 'Ask Yelena about Sara and the DODC', 'Asked Yelena for help. It is possible to be grateful for intelligence and deeply uncomfortable with the location of the meeting at the same time.'),
    (9, 4, 20, 5, 'Recheck Metzger''s account', 'Metzger''s answers are very tidy. New York is never tidy. My apartment is currently providing peer-reviewed evidence.'),
    (9, 5, 23, 4, 'Think about Ned knowing my name', 'Ned accidentally knowing another piece of me isn''t the same as getting the past back. It''s still another piece I don''t have to carry quite so secretly.'),
    (10, 0, 7, 1, 'Prepare to reach Roosevelt Island', 'Checked every piece of equipment twice. Briefly attempted to check my own nervous system. Terrible interface.'),
    (10, 1, 9, 6, 'Take the inhibitor off, keep the backup shooters', 'Organic webs again. The backup shooters are still on me. If my body is improvising, somebody in this outfit should at least have read a manual.'),
    (10, 2, 12, 1, 'Accidentally waterproof the plan with mustard', 'Eating before a mission is sensible. Eating while reading a mission plan is how you laminate the mission plan with mustard.'),
    (10, 3, 16, 5, 'Learn what happened to Sara', 'Sara''s gone. Jean wasn''t searching for a weapon or a prize. She was searching for her sister. There isn''t a gadget that makes finding that out less awful.'),
    (10, 4, 20, 6, 'Use organic webs and new agility against the Hand', 'The Hand brought swords. My body brought more web than the old cartridges could ever hold. Very relieved the adaptation wasn''t something requiring six new sleeves.'),
    (10, 5, 23, 6, 'Learn to read the sharper senses', 'The extra speed and senses finally felt like information I could use instead of a roomful of alarms. Wish the lesson hadn''t required quite this many swords.'),
    (11, 0, 7, 1, 'Find a clean bit of a very dirty suit', 'Tried wiping my mask on the clean bit of the suit. There is no clean bit. I have become a practical demonstration of why people buy a second outfit.'),
    (11, 1, 9, 6, 'Think about the part of me the powers don''t decide', 'Kept worrying about the new powers turning me into something else. They didn''t choose who I stepped in front of. I did.'),
    (11, 2, 12, 1, 'Flatten a protein bar beyond recognition', 'Had a protein bar in the suit. Flattened it beyond recognition. Good news: it now fits in literally any pocket.'),
    (11, 3, 16, 8, 'Let Jean into the memory of May', 'Let Jean see something I normally can''t even say out loud. Harder than letting somebody punch me. Much harder.'),
    (11, 4, 20, 5, 'Shield Jean from Frank''s shot', 'Protected her. Not because she hadn''t hurt people. Because I was there, and I knew what was coming.'),
    (11, 5, 23, 1, 'Leave a page to come back to', 'There should be another entry here. Apparently I have to come back and write it.'),
    (12, 0, 7, 1, 'Review the hospital eggs as an adversary', 'Breakfast arrived under a plastic lid. I have survived a lot this week, but the eggs are making a spirited attempt.'),
    (12, 1, 9, 6, 'Check healing without webbing the room', 'Checked my wrists without firing anything. A landmark day in respecting the property of a building I am already inside.'),
    (12, 2, 12, 1, 'Decline a delivery shift from a hospital bed', 'Phone asked if I''d like to resume delivery work. Body answered with a noise the app wasn''t equipped to classify.'),
    (12, 3, 16, 5, 'Hear Frank''s apology', 'Frank''s version of an apology contains remarkably few decorative words. Didn''t need the decorations.'),
    (12, 4, 20, 5, 'Let somebody else answer the siren', 'Heard a siren and sat up. Several people immediately had opinions about that. Being cared about is surprisingly bossy.'),
    (12, 5, 23, 1, 'Try sleeping without being a hero', 'Put the hospital bracelet beside the suit instead of pretending nothing happened. One of these outfits was free. Guess which one is worse.'),
    (13, 0, 7, 1, 'Fight an allegedly easy-open jar', 'Tried opening a jar while recovering. Organic webs, sharper senses, and somehow still defeated by a product labeled easy-open.'),
    (13, 1, 9, 6, 'Practice accepting the new powers and my limits', 'New powers do not have to mean every emergency belongs exclusively to me. Writing that here because apparently my skull is the one surface I can''t stick a reminder to.'),
    (13, 2, 12, 1, 'Budget without another homemade inhibitor', 'Budget after all this: rent, groceries, no more secret homemade medical devices this month. That last category got out of hand.'),
    (13, 3, 16, 8, 'Tell May I won''t do everything alone', 'Told May I would try not to do everything alone. Immediately thought of six exceptions. Working on it.'),
    (13, 4, 20, 5, 'Let DeWolff handle the robbery', 'Let the police take the robbery. Sat on a roof and ate a sandwich while the extra senses informed me of every moving pigeon in a three-block radius.'),
    (13, 5, 23, 4, 'Meet Ned as Peter, try the handshake', 'Ned laughed. I didn''t need an app notification to know he was there.');
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
WHERE slot<>2 OR day_number%3<>0;
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
SELECT 200,'spider-demo',1,'Recovery, rent, and a very loud spider-sense',
    max(midnight,at-2700),max(midnight,at-2700),max(midnight,at-2700)
FROM spider_clock CROSS JOIN spider_seed WHERE enabled;
INSERT INTO spider_notes
SELECT 225,6,'My wrists make their own webs now. Great. Rent remains external. New senses report every pigeon and siren. Evolution forgot the volume control.',max(midnight,at-2400),NULL FROM spider_clock
UNION ALL
SELECT 226,4,'Ned built an app to find Spider-Man. Used to just text me. I am very proud of his code and taking the entire user experience personally.',max(midnight,at-1500),NULL FROM spider_clock
UNION ALL
SELECT 227,7,'Still thinking about carrying pizza through the rain for MJ and Ned. New agility: impressive. Organic webbing: alarming. Ability to remember taxis exist before dinner becomes a kite: apparently not included in this update.',max(midnight,at-600),NULL FROM spider_clock;
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
