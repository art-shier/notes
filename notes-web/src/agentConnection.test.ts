import {test} from 'node:test';
import assert from 'node:assert/strict';
import {authorizationCode,agentInstruction} from './agentHandoff.ts';

test('authorization deep link survives login and validates public code',()=>{
 assert.equal(authorizationCode('#agent-authorize?code=ABCD-EFG2'),'ABCD-EFG2');
 assert.equal(authorizationCode('#agent-authorize?code=wrong'),'');
 assert.equal(authorizationCode('#other'),null);
});
test('handoff uses this instance for full install and login with human approval',()=>{
 const prompt=agentInstruction('https://my-notes.test');
 assert.ok(prompt.includes('https://my-notes.test/agent/SKILL.md'));
 assert.ok(prompt.includes('https://my-notes.test/agent/install-client.py'));
 assert.ok(prompt.includes('--server "https://my-notes.test"'));
 assert.ok(prompt.includes('--no-browser'));
 assert.ok(prompt.includes('不能代我'));
 assert.ok(!prompt.includes('notes.shier.art'));
});
