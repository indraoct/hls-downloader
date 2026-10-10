Dirty Code
Application to download HLS video and save it into format mp4

Tech Stack:
- Golang
- Html
- CSS
- JS

Based on this script ffmpeg:
```
ffmpeg -i "https://blablalalalalalal.com/qhlMym_V5jAezNLFtigmBw%3D%3D,1791666000/media=hls4/multi=426x240:240,854x480:480,1280x720:720,1400x1080:1080/tX/tXB926/format/1080.mp4.m3u8" -c copy -bsf:a aac_adtstoasc just_video_ffmpeg.mp4
```

I want to create web application to download HLS video from URL m3u8 and save it into format mp4

Guidance:
1. Use Golang to create web application
2. Use Html, CSS, JS to create web interface
3. Use ffmpeg to download HLS video from URL m3u8 and save it into format mp4
4. Use ffmpeg to convert HLS video from URL m3u8 and save it into format mp4
5. There is input URL m3u8, input file name, and button submit, the function download can be multiple file , so it will button add input url and input file name.
6. On input file name after button submit , the text will replace all special characters and space into underscore(_) and then will suffix with "_ffmpeg.mp4"
7. The output file will be on ~/Documents/test absolute path.
8. The process download can be multiple file with golang concurrency
