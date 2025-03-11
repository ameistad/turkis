FROM nginx:alpine
COPY index.html /usr/share/nginx/html/
EXPOSE 80

#HEALTHCHECK --interval=30s --timeout=3s --retries=3 CMD ["sh", "-c", "wget -q --spider http://localhost/ || exit 1"]

CMD ["nginx", "-g", "daemon off;"]
