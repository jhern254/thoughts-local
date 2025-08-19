# Building Go HTMX app

### Potential endpoints
Perfect. Let’s sketch your MVP endpoints directly from the schema you’ve designed. I’ll keep it **practical, minimal, and JSON-first**, so you can evolve later into HTML + htmx without redoing backend logic.

---

# 🚀 Thoughts App — MVP API Endpoints

## 1. **Users**

* **POST /users** → create a user
* **GET /users/\:id** → fetch user profile
* (Optional for MVP: auth, sessions)

---

## 2. **Subjects**

* **POST /subjects** → create a subject (name) for a user
* **GET /subjects** → list all subjects for a user
* **GET /subjects/league** → league endpoint (subject + total thought count) ← *your “wins” equivalent*

---

## 3. **Thoughts**

* **POST /thoughts** → create a thought (`body`, optional `subject_id`, optional `event_id`)
* **GET /thoughts?subject\_id=...** → list thoughts per subject
* **GET /thoughts/\:id** → fetch single thought

---

## 4. **Tags** *(M\:N via thought\_tags)*

* **POST /tags** → create a tag
* **POST /thoughts/\:id/tags** → attach tags to thought
* **GET /tags** → list tags for a user
* **GET /tags/\:id/thoughts** → list all thoughts tagged with a tag

---

## 5. **Events** *(optional in MVP if you want temporal grouping)*

* **POST /events** → create an event (start, end)
* **GET /events** → list events for a user
* **GET /events/\:id/thoughts** → thoughts in an event window

---

## 6. **Goals + Progress**

*(You could defer for a first cut, but schema supports it)*

* **POST /goals** → create a goal (name, target\_seconds, cadence, etc.)
* **GET /goals** → list goals
* **POST /goals/\:id/progress** → add progress (occurrence time, seconds, provenance from thought/event)
* **GET /goals/\:id/rollups** → show rollups (daily/weekly/etc.)
* **GET /goals/\:id/league** → optional — compare goals by accumulated progress

---

## 7. **Rollups (if you materialize)**

* **GET /subjects/\:id/rollups?cadence=weekly** → return subject totals
* **GET /goals/\:id/rollups?cadence=weekly** → return goal totals

---

# 🗂️ Endpoint → Schema Mapping

* `/users` → `users`
* `/subjects` → `subjects`
* `/subjects/league` → `thoughts` joined with `subjects` (aggregate count)
* `/thoughts` → `thoughts`
* `/tags` + `/thoughts/:id/tags` → `tags`, `thought_tags`
* `/events` → `events`
* `/goals` → `goals`
* `/goals/:id/progress` → `goal_progress`
* `/goals/:id/rollups` → `goal_time_rollups`

---

# 🎯 Minimal MVP for demo

If you want to cut scope for a *fastest possible demo*, I’d prioritize:

1. Users
2. Subjects
3. Thoughts
4. League of subjects (thought count)

That alone gives you:

* Add thoughts
* Categorize them into subjects
- add tags, events, and goals later.

---


