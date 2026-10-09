import {defineConfig} from '@playwright/test';
export default defineConfig({testDir:'./tests',workers:1,retries:0,timeout:90000,use:{headless:true,viewport:{width:390,height:844}},reporter:'list'});
