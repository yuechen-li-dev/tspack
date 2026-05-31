const payload = process.argv.slice(2).join(' ');
process.stdout.write(`ready from node ${payload}\n`);
setInterval(() => {}, 1000);
