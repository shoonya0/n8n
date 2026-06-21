# Phase 5: n8n Workflow Configuration

## Objective
Set up the automation flow in the n8n visual editor to receive the incoming payload from our Golang backend worker and process it further to push it to the requested social media platforms.

## 1. Webhook Setup
- **Webhook Node:** Add a Webhook Node as the starting point.
- **Method:** Configure it to accept `POST` requests.
- **Path:** Set a deterministic URL path (e.g., `/social-trigger`).
- **Security:** Ensure IP restrictions or a custom Authentication header logic to only accept requests originating from our Asynq Worker IP or containing a Shared Secret.

## 2. Payload Parsing & Data Manipulation
- Attach a **Set Node** or **Code Node (JavaScript)** to parse the JSON received from the webhook.
- The payload should include the specific target platforms (`linkedin`, `twitter`, etc.) and the `content`.
- Use a **Switch Node** (or IF nodes) to route the data execution flow based on the `platforms` array provided in the JSON payload.

## 3. Social Media Nodes
- **Twitter/X Configuration:**
  - Create a Twitter Node.
  - Setup OAuth2/App credentials inside n8n.
  - Map the `content` text field to the Tweet text input.
- **LinkedIn Configuration:**
  - Create a LinkedIn Node.
  - Setup OAuth2 credentials.
  - Map the `content` text field to the LinkedIn Post content.
- Ensure that you handle individual node failure strategies inside n8n (using Error Trigger nodes) to alert administrators if social media credentials expire.

## 4. Final Testing
- Trigger the entire flow via Postman:
  - Submit POST request to `/api/v1/socialpost`
  - Verify Database correctly stores the entry and marks it `PENDING`.
  - Verify Asynq Worker wakes up, sends webhook, and updates DB to `SENT`.
  - Verify n8n execution log shows a completed run and the final post appears on the target platforms.
