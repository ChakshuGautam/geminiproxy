import express from 'express';
import axios from 'axios';
import bodyParser from 'body-parser';
import fs from 'fs/promises';

const app = express();
const PORT = 4001;
const GEMINI_API_URL = 'https://generativelanguage.googleapis.com/v1beta/models';

app.use(bodyParser.json());

let keys = [];
let keyIndex = 0;

async function init() {
  // Load keys
  const fileContent = await fs.readFile('../../gemini.keys', 'utf-8');
  keys = fileContent
    .split('\n')
    .map(k => k.trim())
    .filter(k => k && !k.startsWith('#'));

  if (keys.length === 0) throw new Error('No API keys found in gemini.keys');

  app.post('/chat/completions', async (req, res) => {
    try {
      const key = keys[keyIndex];
      keyIndex = (keyIndex + 1) % keys.length;

      const response = await axios.post(
        `${GEMINI_API_URL}/${req.body.model}:generateContent?key=${key}`,
        {
          contents: req.body.messages.map(({ role, content }) => ({
            role,
            parts: [{ text: content }]
          }))
        },
        {
          headers: {
            'Content-Type': 'application/json',
          },
        }
      );

      const reply = response.data.candidates[0].content.parts[0].text || '';

      res.json({
        id: 'mock-id',
        object: 'chat.completion',
        choices: [
          {
            message: { role: 'assistant', content: reply },
            finish_reason: 'stop',
            index: 0
          }
        ],
        usage: {}
      });
    } catch (error) {
      console.log(error);
      console.error(error.response.data || error.message);
      res.status(500).json({ error: 'Failed to forward request to Gemini API' });
    }
  });

  app.listen(PORT, () => {
    console.log(`✅ Gemini Proxy running at http://localhost:${PORT}`);
  });
}

init();
